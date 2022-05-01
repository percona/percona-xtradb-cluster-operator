package webhook

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	admission "k8s.io/api/admission/v1"
	admissionregistration "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/cert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	v1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxctls"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/webhook/json"
)

const certPath = "/tmp/k8s-webhook-server/serving-certs/"

var hookPath = "/validate-percona-xtradbcluster"

type hook struct {
	cl        client.Client
	scheme    *runtime.Scheme
	caBundle  []byte
	namespace string
	log       logr.Logger
}

func (h *hook) Start(ctx context.Context) error {
	err := h.setup()
	if err != nil {
		h.log.Info("failed to setup webhook", "err", err.Error())
	}
	<-ctx.Done()
	return nil
}

func (h *hook) setup() error {
	operatorDeployment, err := h.operatorDeployment()
	if err != nil {
		return errors.Wrap(err, "failed to get operator deployment")
	}

	ref, err := k8s.OwnerRef(operatorDeployment, h.scheme)
	if err != nil {
		return errors.Wrap(err, "failed to get deployment owner ref")
	}

	err = h.createService(ref)
	if err != nil {
		return errors.Wrap(err, "Can't create service")
	}

	err = h.createWebhook(ref)
	if err != nil {
		return errors.Wrap(err, "can't create webhook")
	}
	return nil
}

func (h *hook) createService(ownerRef metav1.OwnerReference) error {
	opPod, err := k8s.OperatorPod(h.cl)
	if err != nil {
		return errors.Wrap(err, "get operator pod")
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "percona-xtradb-cluster-operator",
			Namespace:       h.namespace,
			Labels:          map[string]string{"name": "percona-xtradb-cluster-operator"},
			OwnerReferences: []metav1.OwnerReference{ownerRef},
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
			service.ObjectMeta.OwnerReferences = []metav1.OwnerReference{ownerRef}
			return h.cl.Update(context.TODO(), service)
		}
		return err
	}
	return nil
}

func (h *hook) createWebhook(ownerRef metav1.OwnerReference) error {
	failPolicy := admissionregistration.Fail
	sideEffects := admissionregistration.SideEffectClassNone
	hook := &admissionregistration.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "percona-xtradbcluster-webhook",
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Webhooks: []admissionregistration.ValidatingWebhook{
			{
				AdmissionReviewVersions: []string{"v1"},
				Name:                    "validationwebhook.pxc.percona.com",
				ClientConfig: admissionregistration.WebhookClientConfig{
					Service: &admissionregistration.ServiceReference{
						Namespace: h.namespace,
						Name:      "percona-xtradb-cluster-operator",
						Path:      &hookPath,
					},
					CABundle: h.caBundle,
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
	if k8serrors.IsForbidden(err) {
		return nil
	}

	if err != nil && k8serrors.IsAlreadyExists(err) {
		hook := &admissionregistration.ValidatingWebhookConfiguration{}
		err := h.cl.Get(context.TODO(), types.NamespacedName{
			Name: "percona-xtradbcluster-webhook",
		}, hook)
		if err != nil {
			return err
		}

		hook.Webhooks[0].ClientConfig.CABundle = h.caBundle
		hook.ObjectMeta.OwnerReferences = []metav1.OwnerReference{ownerRef}
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

	namespace, err := k8s.GetOperatorNamespace()
	if err != nil {
		return errors.Wrap(err, "get operator namespace")
	}

	ca, err := setupCertificates(mgr.GetAPIReader(), namespace)
	if err != nil {
		return errors.Wrap(err, "prepare hook tls certs")
	}

	zapLog, err := zap.NewProduction()
	if err != nil {
		return errors.Wrap(err, "create logger")
	}

	h := &hook{
		cl:        mgr.GetClient(),
		scheme:    mgr.GetScheme(),
		caBundle:  ca,
		namespace: namespace,
		log:       zapr.NewLogger(zapLog),
	}

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

	bytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		h.log.Error(err, "can't read request body")
		return
	}

	if err := json.Decode(bytes, req, true); err != nil {
		h.log.Error(err, "Can't decode admission review request")
		return
	}

	if req.Request.Kind.Group == "autoscaling" && req.Request.Kind.Kind == "Scale" {
		if err = sendResponse(req.Request.UID, req.TypeMeta, w, nil); err != nil {
			h.log.Error(err, "Can't send validation response")
		}
		return
	}

	cr := &v1.PerconaXtraDBCluster{}
	if err := json.Decode(req.Request.Object.Raw, cr, true); err != nil {
		err = sendResponse(req.Request.UID, req.TypeMeta, w, err)
		if err != nil {
			h.log.Error(err, "Can't send validation response")
		}
		return
	}

	if cr.Spec.EnableCRValidationWebhook == nil || !*cr.Spec.EnableCRValidationWebhook {
		err = sendResponse(req.Request.UID, req.TypeMeta, w, nil)
		if err != nil {
			h.log.Error(err, "Can't send validation response")
		}
		return
	}

	err = sendResponse(req.Request.UID, req.TypeMeta, w, cr.Validate())
	if err != nil {
		h.log.Error(err, "Can't send validation response")
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

func (h *hook) operatorDeployment() (*appsv1.Deployment, error) {
	operatorDeploymentName := os.Getenv("OPERATOR_NAME")
	if operatorDeploymentName == "" {
		operatorDeploymentName = "percona-xtradb-cluster-operator"
	}
	deployment := &appsv1.Deployment{}
	err := h.cl.Get(context.TODO(), types.NamespacedName{
		Name:      operatorDeploymentName,
		Namespace: h.namespace,
	}, deployment)
	return deployment, err
}
