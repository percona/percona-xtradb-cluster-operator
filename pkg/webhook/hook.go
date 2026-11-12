package webhook

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"os"
	"time"

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
	k8sretry "k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxctls"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/webhook/json"
)

const certPath = "/tmp/k8s-webhook-server/serving-certs/"

const (
	// certRenewalThreshold defines how far in advance of expiry the operator
	// proactively reissues the webhook TLS certificate.
	certRenewalThreshold = pxctls.DefaultRenewBefore
	// minCertRenewalRequeueAfter is a safety floor for the reconciler's RequeueAfter,
	// avoiding a tight reconcile loop if something is wrong with the certificate's
	// computed expiry.
	minCertRenewalRequeueAfter = time.Minute
)

var hookPath = "/validate-percona-xtradbcluster"

type hook struct {
	cl        client.Client
	scheme    *runtime.Scheme
	caBundle  []byte
	namespace string
	log       logr.Logger
}

func (h *hook) Start(ctx context.Context) error {
	err := h.setup(ctx)
	if err != nil {
		h.log.Info("failed to setup webhook", "err", err.Error())
	}
	<-ctx.Done()
	return nil
}

func (h *hook) setup(ctx context.Context) error {
	operatorDeployment, err := h.operatorDeployment(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get operator deployment")
	}

	ref, err := k8s.OwnerRef(operatorDeployment, h.scheme)
	if err != nil {
		return errors.Wrap(err, "failed to get deployment owner ref")
	}

	err = h.createService(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "Can't create service")
	}

	err = h.createWebhook(ctx, ref, h.caBundle)
	if err != nil {
		return errors.Wrap(err, "can't create webhook")
	}
	return nil
}

func (h *hook) createService(ctx context.Context, ownerRef metav1.OwnerReference) error {
	opPod, err := k8s.OperatorPod(ctx, h.cl)
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
	err = h.cl.Create(ctx, svc)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			service := &corev1.Service{}
			err = h.cl.Get(ctx, types.NamespacedName{
				Name:      "percona-xtradb-cluster-operator",
				Namespace: h.namespace,
			}, service)
			if err != nil {
				return err
			}

			service.Spec.Selector = opPod.Labels
			service.ObjectMeta.OwnerReferences = []metav1.OwnerReference{ownerRef}
			return h.cl.Update(ctx, service)
		}
		return err
	}
	return nil
}

const webhookConfigName = "percona-xtradbcluster-webhook"

func (h *hook) createWebhook(ctx context.Context, ownerRef metav1.OwnerReference, caBundle []byte) error {
	failPolicy := admissionregistration.Fail
	sideEffects := admissionregistration.SideEffectClassNone
	wh := &admissionregistration.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:            webhookConfigName,
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
					CABundle: caBundle,
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

	err := h.cl.Create(ctx, wh)
	if k8serrors.IsForbidden(err) {
		return nil
	}

	if err != nil && k8serrors.IsAlreadyExists(err) {
		existing := &admissionregistration.ValidatingWebhookConfiguration{}
		err := h.cl.Get(ctx, types.NamespacedName{
			Name: webhookConfigName,
		}, existing)
		if err != nil {
			return err
		}

		// skip if the webhook was already reconciled by another replica
		if len(existing.Webhooks) > 0 && bytes.Equal(existing.Webhooks[0].ClientConfig.CABundle, caBundle) {
			return nil
		}

		if len(existing.Webhooks) == 0 {
			existing.Webhooks = wh.Webhooks
		} else {
			existing.Webhooks[0].ClientConfig.CABundle = caBundle
		}
		existing.ObjectMeta.OwnerReferences = []metav1.OwnerReference{ownerRef}
		return h.cl.Update(ctx, existing)
	}
	return err
}

// SetupWebhook prepares certificates for webhook and
// create ValidatingWebhookConfiguration k8s object
func SetupWebhook(ctx context.Context, mgr manager.Manager) error {
	err := admissionregistration.AddToScheme(mgr.GetScheme())
	if err != nil {
		return errors.Wrap(err, "add admissionregistration to scheme")
	}

	namespace, err := k8s.GetOperatorNamespace()
	if err != nil {
		return errors.Wrap(err, "get operator namespace")
	}

	zapLog, err := zap.NewProduction()
	if err != nil {
		return errors.Wrap(err, "create logger")
	}
	log := zapr.NewLogger(zapLog)

	// mgr.GetClient() reads go through the informer cache, which starts only inside
	// mgr.Start(). SetupWebhook runs earlier, so use the API reader for Secret Get.
	ca, _, err := ensureCertificates(ctx, mgr.GetAPIReader(), mgr.GetClient(), namespace, log)
	if err != nil {
		return errors.Wrap(err, "prepare hook tls certs")
	}

	h := &hook{
		cl:        mgr.GetClient(),
		scheme:    mgr.GetScheme(),
		caBundle:  ca,
		namespace: namespace,
		log:       log,
	}

	mgr.GetWebhookServer().Register(hookPath, h)

	err = mgr.Add(h)
	if err != nil {
		return errors.Wrap(err, "add webhook creator to manager")
	}

	if err := setupCertSyncController(mgr, h, namespace, log); err != nil {
		return errors.Wrap(err, "setup webhook cert sync controller")
	}

	return nil
}

func isWebhookTLSSecret(namespace string) func(obj client.Object) bool {
	return func(obj client.Object) bool {
		return obj.GetNamespace() == namespace && obj.GetName() == naming.OperatorWebhookTLSSecretName
	}
}

// certSyncer watches the webhook TLS secret and runs on every operator
// replica, keeps cert files in sync with the Secret. Only the elected leader renews the
// certificate and keeps the ValidatingWebhookConfiguration's CABundle in sync.
// elected is the channel returned by manager.Manager.Elected(), closed once this
// replica becomes leader or immediately, if leader election is disabled
type certSyncer struct {
	hook      *hook
	namespace string
	log       logr.Logger
	elected   <-chan struct{}
}

func (s *certSyncer) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	certSecret := &corev1.Secret{}
	err := s.hook.cl.Get(ctx, types.NamespacedName{Namespace: s.namespace, Name: naming.OperatorWebhookTLSSecretName}, certSecret)
	switch {
	case k8serrors.IsNotFound(err):
		// Nothing to sync yet; the leader hasn't issued the certificate yet.
	case err != nil:
		return reconcile.Result{}, errors.Wrap(err, "get cert secret")
	default:
		crt, ok := certSecret.Data["tls.crt"]
		if !ok {
			return reconcile.Result{}, errors.New("tls.crt not found in cert secret")
		}
		key, ok := certSecret.Data["tls.key"]
		if !ok {
			return reconcile.Result{}, errors.New("tls.key not found in cert secret")
		}
		if err := writeCerts(crt, key); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "sync local webhook tls cert files")
		}
	}

	select {
	case <-s.elected:
		// We're the leader: fall through to renew/sync below.
	default:
		// Not the leader (yet): only the file sync above applies to us.
		return reconcile.Result{}, nil
	}

	ca, notAfter, err := ensureCertificates(ctx, s.hook.cl, s.hook.cl, s.namespace, s.log)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "ensure webhook tls certificates")
	}

	operatorDeployment, err := s.hook.operatorDeployment(ctx)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "get operator deployment")
	}

	ref, err := k8s.OwnerRef(operatorDeployment, s.hook.scheme)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "get deployment owner ref")
	}

	if err := s.hook.createWebhook(ctx, ref, ca); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "update ValidatingWebhookConfiguration CABundle")
	}

	return reconcile.Result{RequeueAfter: certRenewalRequeueAfter(notAfter)}, nil
}

func (s *certSyncer) NeedLeaderElection() bool {
	return false
}

func setupCertSyncController(mgr manager.Manager, h *hook, namespace string, log logr.Logger) error {
	s := &certSyncer{
		hook:      h,
		namespace: namespace,
		log:       log,
		elected:   mgr.Elected(),
	}

	return builder.ControllerManagedBy(mgr).
		Named("webhook-cert-sync").
		For(&corev1.Secret{}, builder.WithPredicates(predicate.NewPredicateFuncs(isWebhookTLSSecret(namespace)))).
		WithOptions(controller.Options{NeedLeaderElection: ptr.To(false)}).
		Complete(s)
}

func certRenewalRequeueAfter(notAfter time.Time) time.Duration {
	d := time.Until(notAfter) - certRenewalThreshold
	if d < minCertRenewalRequeueAfter {
		return minCertRenewalRequeueAfter
	}
	return d
}

func ensureCertificates(
	ctx context.Context,
	reader client.Reader,
	cl client.Client,
	namespace string,
	log logr.Logger,
) ([]byte, time.Time, error) {
	var (
		ca       []byte
		notAfter time.Time
	)

	// Retry on AlreadyExists/Conflict errors.
	// This typically happens when multiple replicas start/reconcile at once and race to
	// create or update the same secret. The ones that fail will re-attempt, re-reading the
	// (now existing/updated) secret.
	if err := k8sretry.OnError(k8sretry.DefaultBackoff, func(err error) bool {
		return k8serrors.IsAlreadyExists(err) || k8serrors.IsConflict(err)
	}, func() error {
		var setupErr error
		ca, notAfter, setupErr = setupCertificates(ctx, reader, cl, namespace, log)
		return setupErr
	}); err != nil {
		return nil, time.Time{}, errors.Wrap(err, "ensure certificates")
	}

	return ca, notAfter, nil
}

func setupCertificates(
	ctx context.Context,
	reader client.Reader,
	cl client.Client,
	namespace string,
	log logr.Logger,
) ([]byte, time.Time, error) {
	certSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      naming.OperatorWebhookTLSSecretName,
		},
	}

	// Use the provided reader (the API reader on first startup, before the informer
	// cache has synced, or the cached client on subsequent reconciles).
	err := reader.Get(ctx, client.ObjectKeyFromObject(certSecret), certSecret)
	if k8serrors.IsNotFound(err) {
		// Secret does not exist, issue new certificates and create the secret
		return issueAndStoreCertificates(ctx, cl, certSecret, namespace)
	} else if err != nil {
		return nil, time.Time{}, errors.Wrap(err, "get cert secret")
	}

	// Secret exists, use the existing certificates unless they are expired or
	// about to expire.
	ca, ok := certSecret.Data["ca.crt"]
	if !ok {
		return nil, time.Time{}, errors.New("ca.crt not found in cert secret")
	}
	crt, ok := certSecret.Data["tls.crt"]
	if !ok {
		return nil, time.Time{}, errors.New("tls.crt not found in cert secret")
	}
	key, ok := certSecret.Data["tls.key"]
	if !ok {
		return nil, time.Time{}, errors.New("tls.key not found in cert secret")
	}

	notAfter, err := certNotAfter(crt)
	if err != nil {
		log.Info("failed to parse existing webhook TLS certificate, reissuing", "error", err.Error())
		return issueAndStoreCertificates(ctx, cl, certSecret, namespace)
	}

	timeLeft := time.Until(notAfter)
	if timeLeft <= certRenewalThreshold {
		if timeLeft <= 0 {
			log.Info("webhook TLS certificate has expired, reissuing", "notAfter", notAfter)
		} else {
			log.Info("webhook TLS certificate is about to expire, reissuing",
				"notAfter", notAfter, "timeLeft", timeLeft.String())
		}
		return issueAndStoreCertificates(ctx, cl, certSecret, namespace)
	}

	if err := writeCerts(crt, key); err != nil {
		return nil, time.Time{}, err
	}
	return ca, notAfter, nil
}

func issueAndStoreCertificates(
	ctx context.Context,
	cl client.Client,
	certSecret *corev1.Secret,
	namespace string,
) ([]byte, time.Time, error) {
	ca, crt, key, err := pxctls.Issue([]string{"percona-xtradb-cluster-operator." + namespace + ".svc"}, true, true)
	if err != nil {
		return nil, time.Time{}, errors.Wrap(err, "issue tls certificates")
	}

	certSecret.Data = map[string][]byte{
		"ca.crt":  ca,
		"tls.crt": crt,
		"tls.key": key,
	}

	if len(certSecret.ResourceVersion) > 0 {
		err = cl.Update(ctx, certSecret)
	} else {
		err = cl.Create(ctx, certSecret)
	}
	if err != nil {
		return nil, time.Time{}, errors.Wrap(err, "store cert secret")
	}

	if err := writeCerts(crt, key); err != nil {
		return nil, time.Time{}, err
	}

	notAfter, err := certNotAfter(crt)
	if err != nil {
		return nil, time.Time{}, errors.Wrap(err, "parse issued certificate")
	}

	return ca, notAfter, nil
}

// certNotAfter parses a PEM-encoded certificate and returns its NotAfter (expiry) time.
func certNotAfter(crtPEM []byte) (time.Time, error) {
	block, _ := pem.Decode(crtPEM)
	if block == nil {
		return time.Time{}, errors.New("failed to decode PEM block containing certificate")
	}

	crt, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, errors.Wrap(err, "parse certificate")
	}

	return crt.NotAfter, nil
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

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.log.Error(err, "can't read request body")
		return
	}

	if err := json.Decode(body, req, true); err != nil {
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

func (h *hook) operatorDeployment(ctx context.Context) (*appsv1.Deployment, error) {
	operatorDeploymentName := os.Getenv("OPERATOR_NAME")
	if operatorDeploymentName == "" {
		operatorDeploymentName = "percona-xtradb-cluster-operator"
	}
	deployment := &appsv1.Deployment{}
	err := h.cl.Get(ctx, types.NamespacedName{
		Name:      operatorDeploymentName,
		Namespace: h.namespace,
	}, deployment)
	return deployment, err
}
