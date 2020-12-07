package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/pkg/errors"
	admission "k8s.io/api/admission/v1"
	admissionregistration "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/cert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	log "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	v1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxctls"
)

const certPath = "/tmp/k8s-webhook-server/serving-certs/"

var hookPath = "/validate-percona-xtradbcluster"

type hook struct {
	cl        client.Client
	caBunlde  []byte
	namespace string
}

func (h *hook) Start(i <-chan struct{}) error {
	err := h.createService()
	if err != nil {
		log.Log.Info("Can't create service", "err", err.Error())
	}
	err = h.createWebhook()
	if err != nil {
		log.Log.Info("Can't create webhook", "error", err.Error())
	}
	<-i
	return nil
}

func (h *hook) createService() error {
	opPod, err := k8s.OperatorPod(h.cl)
	if err != nil {
		return errors.Wrap(err, "get operator pod")
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "percona-xtradb-cluster-operator",
			Namespace: h.namespace,
			Labels:    map[string]string{"name": "percona-xtradb-cluster-operator"},
		},
		Spec: corev1.ServiceSpec{
			Ports:    []corev1.ServicePort{{Port: 443, TargetPort: intstr.FromInt(9443)}},
			Selector: opPod.Labels,
		},
	}
	err = h.cl.Create(context.TODO(), svc)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			service := &corev1.Service{}
			err = h.cl.Get(context.TODO(), types.NamespacedName{
				Name:      "percona-xtradb-cluster-operator",
				Namespace: h.namespace,
			}, service)
			if err != nil {
				return err
			}

			service.Spec.Selector = opPod.Labels
			return h.cl.Update(context.TODO(), service)
		}
		return err
	}
	return nil
}

func (h *hook) createWebhook() error {
	failPolicy := admissionregistration.Fail
	sideEffects := admissionregistration.SideEffectClassNone
	hook := &admissionregistration.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "percona-xtradbcluster-webhook",
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
							Resources:   []string{"perconaxtradbclusters/*"},
						},
						Operations: []admissionregistration.OperationType{"CREATE", "UPDATE"},
					},
				},
			},
		},
	}
	err := h.cl.Create(context.TODO(), hook)
	if err != nil && k8serrors.IsAlreadyExists(err) {
		hook := &admissionregistration.ValidatingWebhookConfiguration{}
		err := h.cl.Get(context.TODO(), types.NamespacedName{
			Name: "percona-xtradbcluster-webhook",
		}, hook)
		if err != nil {
			return err
		}

		hook.Webhooks[0].ClientConfig.CABundle = h.caBunlde
		return h.cl.Update(context.TODO(), hook)
	}
	return err
}

// SetupWebhook prepares certificates for webhook and
// create ValidatingWebhookConfiguration k8s object
func SetupWebhook(mgr manager.Manager) error {
	err := admissionregistration.AddToScheme(mgr.GetScheme())
	if err != nil {
		return errors.Wrap(err, "add admissionregistration to scheme")
	}

	namespace, err := k8sutil.GetOperatorNamespace()
	if err != nil {
		return errors.Wrap(err, "get operator namespace")
	}

	ca, err := setupCertificates(mgr.GetAPIReader(), namespace)
	if err != nil {
		return errors.Wrap(err, "prepare hook tls certs")
	}

	h := &hook{cl: mgr.GetClient(), caBunlde: ca, namespace: namespace}

	srv := mgr.GetWebhookServer()
	srv.Port = 9443
	srv.CertDir = certPath
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

	if k8serrors.IsNotFound(err) {
		ca, crt, key, err = pxctls.Issue([]string{"percona-xtradb-cluster-operator." + namespace + ".svc"})
		if err != nil {
			return nil, errors.Wrap(err, "issue tls certificates")
		}
	} else {
		ca, crt, key = certSecret.Data["ca.crt"], certSecret.Data["tls.crt"], certSecret.Data["tls.key"]
	}

	return ca, writeCerts(crt, key)
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
		log.Log.Error(err, "Can't decode admission review request")
		return
	}

	cr := &v1.PerconaXtraDBCluster{}
	decoder := json.NewDecoder(bytes.NewReader(req.Request.Object.Raw))
	decoder.DisallowUnknownFields()

	err = decoder.Decode(cr)
	if err != nil {
		err = sendResponse(req.Request.UID, req.TypeMeta, w, err)
		if err != nil {
			log.Log.Error(err, "Can't send validation response")
		}
		return
	}

	if cr.Spec.DisableHookValidation {
		err = sendResponse(req.Request.UID, req.TypeMeta, w, nil)
		if err != nil {
			log.Log.Error(err, "Can't send validation response")
		}
		return
	}

	err = sendResponse(req.Request.UID, req.TypeMeta, w, cr.Validate())
	if err != nil {
		log.Log.Error(err, "Can't send validation response")
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
			Code:    403,
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
