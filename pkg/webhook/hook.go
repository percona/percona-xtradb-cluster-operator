package webhook

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
	admission "k8s.io/api/admission/v1"
	admissionregistration "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/cert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	v1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxctls"
)

const certPath = "/tmp/k8s-webhook-server/serving-certs/"

var hookPath = "/validate-pxc-percona-com"

type hook struct {
	cl        client.Client
	caBunlde  []byte
	namespace string
}

func (h *hook) Start(i <-chan struct{}) error {
	err := h.createWebhook()
	if err != nil {
		logf.Log.Info("Can't create webhook", "error", err.Error())
	}
	<-i
	return nil
}

func (h *hook) createWebhook() error {
	failPolicy := admissionregistration.Fail
	sideEffects := admissionregistration.SideEffectClassNone
	hook := &admissionregistration.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pxc-validation-webhook",
		},
		Webhooks: []admissionregistration.ValidatingWebhook{
			{
				AdmissionReviewVersions: []string{"v1", "v1beta1"},
				Name:                    "validationwebhook.pxc.percona.com",
				ClientConfig: admissionregistration.WebhookClientConfig{
					Service: &admissionregistration.ServiceReference{
						Namespace: h.namespace,
						Name:      "percona-xtradb-cluster-operator",
						Path:      &hookPath,
					},
					CABundle: h.caBunlde,
				},
				SideEffects:   &sideEffects,
				FailurePolicy: &failPolicy,
				Rules: []admissionregistration.RuleWithOperations{
					{
						Rule: admissionregistration.Rule{
							APIGroups:   []string{"pxc.percona.com"},
							APIVersions: []string{"*"},
							Resources:   []string{"*/*"},
						},
						Operations: []admissionregistration.OperationType{"CREATE", "UPDATE"},
					},
				},
			},
		},
	}
	err := h.cl.Create(context.TODO(), hook)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			err = h.cl.Delete(context.TODO(), hook)
			if err != nil {
				return err
			}
			return h.cl.Create(context.TODO(), hook)
		}
		return err
	}
	return nil
}

// SetupWebhook prepares certificates for webhook and
// create ValidatingWebhookConfiguration k8s object
func SetupWebhook(mgr manager.Manager) error {
	err := admissionregistration.AddToScheme(mgr.GetScheme())
	if err != nil {
		return errors.Wrap(err, "add admissionregistration to scheme")
	}

	nsBytes, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return errors.Wrap(err, "read namespace name from file")
	}

	ca, err := setupCertificates(mgr.GetAPIReader(), string(nsBytes))
	if err != nil {
		return errors.Wrap(err, "prepare hook tls certs")
	}

	h := &hook{cl: mgr.GetClient(), caBunlde: ca, namespace: string(nsBytes)}

	srv := mgr.GetWebhookServer()
	srv.Port = 9443
	srv.Register(hookPath, h)

	err = mgr.Add(h)
	if err != nil {
		return errors.Wrap(err, "add webhook creator to manager")
	}

	return nil
}

func setupCertificates(cl client.Reader, namespace string) ([]byte, error) {
	certSecret := &corev1.Secret{}
	err := cl.Get(context.TODO(), types.NamespacedName{
		Namespace: namespace,
		Name:      "pxc-webhook-ssl",
	}, certSecret)
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, err
	}
	var ca, crt, key []byte
	if err != nil {
		ca, crt, key, err = issueCertificates(namespace)
		if err != nil {
			return nil, err
		}
	} else {
		ca, crt, key = certSecret.Data["ca.crt"], certSecret.Data["tls.crt"], certSecret.Data["tls.key"]
	}
	return ca, writeCerts(crt, key)
}

func issueCertificates(namespace string) ([]byte, []byte, []byte, error) {
	ca, crt, key, err := pxctls.Issue([]string{"percona-xtradb-cluster-operator." + namespace + ".svc"})
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "issue tls certificate")
	}
	return ca, crt, key, nil
}

func writeCerts(crt, key []byte) error {
	err := cert.WriteCert(certPath+"tls.crt", crt)
	if err != nil {
		return errors.Wrap(err, "write tls.crt")
	}
	err = cert.WriteCert(certPath+"tls.key", key)
	if err != nil {
		return errors.Wrap(err, "write tls.key")
	}
	return nil
}

func (h *hook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	req := &admission.AdmissionReview{}

	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		logf.Log.Error(err, "Can't decode admission review request")
		return
	}

	cr := &v1.PerconaXtraDBCluster{}
	err = json.Unmarshal(req.Request.Object.Raw, &cr)
	if err != nil {
		sendResponse(req.Request.UID, req.TypeMeta, w, err)
		return
	}

	err = sendResponse(req.Request.UID, req.TypeMeta, w, cr.Validate())
	if err != nil {
		logf.Log.Error(err, "Can't send validation response")
	}
}

func sendResponse(uid types.UID, meta metav1.TypeMeta, w http.ResponseWriter, err error) error {
	resp := &admission.AdmissionReview{
		TypeMeta: meta,
		Response: &admission.AdmissionResponse{
			UID:     uid,
			Allowed: true,
		},
	}
	if err != nil {
		resp.Response.Allowed = false
		resp.Response.Result = &metav1.Status{
			Message: err.Error(),
		}
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return errors.Wrap(err, "marshall response")
	}
	w.Header().Add("Content-Type", "application/json")
	_, err = w.Write(data)
	if err != nil {
		return errors.Wrap(err, "write response")
	}
	return nil
}
