package webhook

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistration "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake" //nolint
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxctls"
)

const testNamespace = "pxc-operator"

func newFakeClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(objs...).
		Build()
}

func TestSetupCertificates_CreatesSecretWhenMissing(t *testing.T) {
	cl := newFakeClient()

	ca, notAfter, err := setupCertificates(context.Background(), cl, cl, testNamespace, logr.Discard())
	require.NoError(t, err)
	assert.NotEmpty(t, ca)
	assert.True(t, notAfter.After(time.Now()))

	secret := &corev1.Secret{}
	err = cl.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: naming.OperatorWebhookTLSSecretName}, secret)
	require.NoError(t, err)
	assert.Equal(t, ca, secret.Data["ca.crt"])
	assert.NotEmpty(t, secret.Data["tls.crt"])
	assert.NotEmpty(t, secret.Data["tls.key"])
}

func TestSetupCertificates_ReusesValidCertificate(t *testing.T) {
	ca, crt, key, err := pxctls.Issue([]string{"percona-xtradb-cluster-operator." + testNamespace + ".svc"}, true, true)
	require.NoError(t, err)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      naming.OperatorWebhookTLSSecretName,
		},
		Data: map[string][]byte{
			"ca.crt":  ca,
			"tls.crt": crt,
			"tls.key": key,
		},
	}

	cl := newFakeClient(secret)

	gotCA, _, err := setupCertificates(context.Background(), cl, cl, testNamespace, logr.Discard())
	require.NoError(t, err)
	assert.Equal(t, ca, gotCA, "existing valid certificate should be reused")

	// Secret in the cluster should be unchanged.
	stored := &corev1.Secret{}
	require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(secret), stored))
	assert.Equal(t, crt, stored.Data["tls.crt"])
}

func TestSetupCertificates_ReissuesExpiredCertificate(t *testing.T) {
	expiredCA, expiredCrt, expiredKey := buildCertWithNotAfter(t, time.Now().Add(-time.Hour))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      naming.OperatorWebhookTLSSecretName,
		},
		Data: map[string][]byte{
			"ca.crt":  expiredCA,
			"tls.crt": expiredCrt,
			"tls.key": expiredKey,
		},
	}

	cl := newFakeClient(secret)

	gotCA, notAfter, err := setupCertificates(context.Background(), cl, cl, testNamespace, logr.Discard())
	require.NoError(t, err)
	assert.False(t, bytes.Equal(expiredCA, gotCA), "expired certificate should be reissued")
	assert.True(t, time.Until(notAfter) > certRenewalThreshold, "new certificate should have a fresh expiry")

	stored := &corev1.Secret{}
	require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(secret), stored))
	assert.False(t, bytes.Equal(expiredCrt, stored.Data["tls.crt"]))

	storedNotAfter, err := certNotAfter(stored.Data["tls.crt"])
	require.NoError(t, err)
	assert.True(t, time.Until(storedNotAfter) > certRenewalThreshold, "new certificate should have a fresh expiry")
}

func TestSetupCertificates_ReissuesSoonToExpireCertificate(t *testing.T) {
	soonExpiringCA, soonExpiringCrt, soonExpiringKey := buildCertWithNotAfter(t, time.Now().Add(certRenewalThreshold/2))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      naming.OperatorWebhookTLSSecretName,
		},
		Data: map[string][]byte{
			"ca.crt":  soonExpiringCA,
			"tls.crt": soonExpiringCrt,
			"tls.key": soonExpiringKey,
		},
	}

	cl := newFakeClient(secret)

	gotCA, _, err := setupCertificates(context.Background(), cl, cl, testNamespace, logr.Discard())
	require.NoError(t, err)
	assert.False(t, bytes.Equal(soonExpiringCA, gotCA), "soon to expire certificate should be reissued")
}

func TestCertRenewalRequeueAfter(t *testing.T) {
	// Far in the future: requeue roughly at notAfter - threshold.
	notAfter := time.Now().Add(100 * 24 * time.Hour)
	got := certRenewalRequeueAfter(notAfter)
	want := time.Until(notAfter) - certRenewalThreshold
	assert.InDelta(t, want, got, float64(time.Second))

	// Already within the renewal window / past expiry: floor applies.
	assert.Equal(t, minCertRenewalRequeueAfter, certRenewalRequeueAfter(time.Now().Add(-time.Hour)))
}

func TestCreateWebhook_CreatesAndUpdatesCABundle(t *testing.T) {
	cl := newFakeClient()
	h := &hook{cl: cl, namespace: testNamespace}
	ownerRef := metav1.OwnerReference{Name: "percona-xtradb-cluster-operator", Kind: "Deployment", APIVersion: "apps/v1", UID: "test-uid"}

	require.NoError(t, h.createWebhook(context.Background(), ownerRef, []byte("ca-v1")))

	wh := &admissionregistration.ValidatingWebhookConfiguration{}
	require.NoError(t, cl.Get(context.Background(), client.ObjectKey{Name: webhookConfigName}, wh))
	require.Len(t, wh.Webhooks, 1)
	assert.Equal(t, []byte("ca-v1"), wh.Webhooks[0].ClientConfig.CABundle)
	resourceVersionAfterCreate := wh.ResourceVersion

	// Calling again with the same CABundle should be a no-op (no unnecessary update).
	require.NoError(t, h.createWebhook(context.Background(), ownerRef, []byte("ca-v1")))
	require.NoError(t, cl.Get(context.Background(), client.ObjectKey{Name: webhookConfigName}, wh))
	assert.Equal(t, resourceVersionAfterCreate, wh.ResourceVersion, "CABundle unchanged, should not update")

	// Calling with a new CABundle should update it.
	require.NoError(t, h.createWebhook(context.Background(), ownerRef, []byte("ca-v2")))
	require.NoError(t, cl.Get(context.Background(), client.ObjectKey{Name: webhookConfigName}, wh))
	assert.Equal(t, []byte("ca-v2"), wh.Webhooks[0].ClientConfig.CABundle)
}

// newOperatorDeployment returns the Deployment that hook.operatorDeployment looks up
// (by the default name, since OPERATOR_NAME is unset in tests), needed to compute the
// owner reference for the Secret/ValidatingWebhookConfiguration.
func newOperatorDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "percona-xtradb-cluster-operator",
			Namespace: testNamespace,
			UID:       "operator-deployment-uid",
		},
	}
}

// closedChan returns an already-closed channel, simulating a replica that holds
// (or always held, since leader election is disabled) leadership - see
// manager.Manager.Elected().
func closedChan() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func TestCertSyncer_Leader_ReissuesExpiredCertificateAndSyncsCABundle(t *testing.T) {
	expiredCA, expiredCrt, expiredKey := buildCertWithNotAfter(t, time.Now().Add(-time.Hour))
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      naming.OperatorWebhookTLSSecretName,
		},
		Data: map[string][]byte{
			"ca.crt":  expiredCA,
			"tls.crt": expiredCrt,
			"tls.key": expiredKey,
		},
	}

	cl := newFakeClient(secret, newOperatorDeployment())
	h := &hook{cl: cl, scheme: scheme.Scheme, namespace: testNamespace, log: logr.Discard()}
	s := &certSyncer{hook: h, namespace: testNamespace, log: logr.Discard(), elected: closedChan()}

	res, err := s.Reconcile(context.Background(), reconcile.Request{})
	require.NoError(t, err)

	// The secret should now hold a freshly issued, non-expired certificate.
	stored := &corev1.Secret{}
	require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(secret), stored))
	assert.False(t, bytes.Equal(expiredCrt, stored.Data["tls.crt"]), "expired certificate should have been reissued")

	storedNotAfter, err := certNotAfter(stored.Data["tls.crt"])
	require.NoError(t, err)
	assert.True(t, time.Until(storedNotAfter) > certRenewalThreshold)

	// The ValidatingWebhookConfiguration's CABundle should be updated to match the new CA.
	wh := &admissionregistration.ValidatingWebhookConfiguration{}
	require.NoError(t, cl.Get(context.Background(), types.NamespacedName{Name: webhookConfigName}, wh))
	require.Len(t, wh.Webhooks, 1)
	assert.Equal(t, stored.Data["ca.crt"], wh.Webhooks[0].ClientConfig.CABundle)

	// RequeueAfter should reflect the new certificate's fresh expiry, not the floor.
	assert.Greater(t, res.RequeueAfter, minCertRenewalRequeueAfter)
}

func TestCertSyncer_Leader_ValidCertificateIsNotTouchedButCABundleIsSynced(t *testing.T) {
	ca, crt, key, err := pxctls.Issue([]string{"percona-xtradb-cluster-operator." + testNamespace + ".svc"}, true, true)
	require.NoError(t, err)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      naming.OperatorWebhookTLSSecretName,
		},
		Data: map[string][]byte{
			"ca.crt":  ca,
			"tls.crt": crt,
			"tls.key": key,
		},
	}

	// Simulate a webhook config that still points at a stale/different CA
	// (e.g. left over from before the secret was last rotated).
	existingWebhook := &admissionregistration.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: webhookConfigName},
		Webhooks: []admissionregistration.ValidatingWebhook{
			{ClientConfig: admissionregistration.WebhookClientConfig{CABundle: []byte("stale-ca")}},
		},
	}

	cl := newFakeClient(secret, newOperatorDeployment(), existingWebhook)
	h := &hook{cl: cl, scheme: scheme.Scheme, namespace: testNamespace, log: logr.Discard()}
	s := &certSyncer{hook: h, namespace: testNamespace, log: logr.Discard(), elected: closedChan()}

	_, err = s.Reconcile(context.Background(), reconcile.Request{})
	require.NoError(t, err)

	// The valid certificate should not have been reissued.
	stored := &corev1.Secret{}
	require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(secret), stored))
	assert.Equal(t, crt, stored.Data["tls.crt"], "valid certificate should be left untouched")

	// But the webhook's CABundle should now be synced to match it.
	wh := &admissionregistration.ValidatingWebhookConfiguration{}
	require.NoError(t, cl.Get(context.Background(), types.NamespacedName{Name: webhookConfigName}, wh))
	assert.Equal(t, ca, wh.Webhooks[0].ClientConfig.CABundle)
}

func TestCertSyncer_NonLeader_OnlySyncsLocalFilesAndSkipsRenewalAndCABundle(t *testing.T) {
	ca, crt, key, err := pxctls.Issue([]string{"percona-xtradb-cluster-operator." + testNamespace + ".svc"}, true, true)
	require.NoError(t, err)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      naming.OperatorWebhookTLSSecretName,
		},
		Data: map[string][]byte{
			"ca.crt":  ca,
			"tls.crt": crt,
			"tls.key": key,
		},
	}

	cl := newFakeClient(secret, newOperatorDeployment())
	h := &hook{cl: cl, scheme: scheme.Scheme, namespace: testNamespace, log: logr.Discard()}
	// Never-closed channel: this replica is not (yet) the leader.
	s := &certSyncer{hook: h, namespace: testNamespace, log: logr.Discard(), elected: make(chan struct{})}

	res, err := s.Reconcile(context.Background(), reconcile.Request{})
	require.NoError(t, err)
	assert.Zero(t, res.RequeueAfter, "non-leader should not schedule a renewal requeue")

	// Non-leader must not touch the Secret or create the ValidatingWebhookConfiguration.
	stored := &corev1.Secret{}
	require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(secret), stored))
	assert.Equal(t, crt, stored.Data["tls.crt"])

	wh := &admissionregistration.ValidatingWebhookConfiguration{}
	err = cl.Get(context.Background(), types.NamespacedName{Name: webhookConfigName}, wh)
	assert.Error(t, err, "non-leader must not create/update the ValidatingWebhookConfiguration")
}

func TestCertNotAfter(t *testing.T) {
	want := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	_, crt, _ := buildCertWithNotAfter(t, want)

	got, err := certNotAfter(crt)
	require.NoError(t, err)
	assert.WithinDuration(t, want, got, time.Second)
}

func TestCertNotAfter_InvalidPEM(t *testing.T) {
	_, err := certNotAfter([]byte("not a cert"))
	assert.Error(t, err)
}

// buildCertWithNotAfter builds a minimal self-signed CA certificate and a leaf certificate
// signed by it with the given NotAfter time, returning the PEM-encoded CA cert, leaf cert and
// leaf private key.
func buildCertWithNotAfter(t *testing.T, notAfter time.Time) (caPEM, crtPEM, keyPEM []byte) {
	t.Helper()

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	caTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"Root CA"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              notAfter.Add(time.Hour * 24 * 365),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)

	caBuf := &bytes.Buffer{}
	require.NoError(t, pem.Encode(caBuf, &pem.Block{Type: "CERTIFICATE", Bytes: caDER}))

	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	leafTemplate := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{Organization: []string{"PXC"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		DNSNames:     []string{"percona-xtradb-cluster-operator." + testNamespace + ".svc"},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	leafDER, err := x509.CreateCertificate(rand.Reader, &leafTemplate, &caTemplate, &leafKey.PublicKey, caKey)
	require.NoError(t, err)

	leafBuf := &bytes.Buffer{}
	require.NoError(t, pem.Encode(leafBuf, &pem.Block{Type: "CERTIFICATE", Bytes: leafDER}))

	keyBuf := &bytes.Buffer{}
	require.NoError(t, pem.Encode(keyBuf, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(leafKey)}))

	return caBuf.Bytes(), leafBuf.Bytes(), keyBuf.Bytes()
}
