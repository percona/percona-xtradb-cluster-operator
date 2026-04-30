package pxc

import (
	"bytes"
	"testing"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/apis"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestRotateSSLCertificate(t *testing.T) {
	ctx := t.Context()
	cr, err := readDefaultCR("test-cluster", "default")
	require.NoError(t, err)
	err = cr.CheckNSetDefaults(new(version.ServerVersion), logf.FromContext(ctx))
	require.NoError(t, err)

	sts := statefulset.NewNode(cr).StatefulSet()
	internalSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.PXC.SSLInternalSecretName,
			Namespace: cr.GetNamespace(),
		},
		Data: map[string][]byte{
			"ca.crt":  []byte("test-ca-crt"),
			"tls.crt": []byte("test-tls-crt"),
			"tls.key": []byte("test-tls-key"),
		},
	}
	newSecret := internalSecret.DeepCopy()
	newSecret.Name = cr.Spec.PXC.SSLInternalSecretName + "-new"
	newSecret.Data = map[string][]byte{
		"ca.crt":  []byte("test-ca-crt-new"),
		"tls.crt": []byte("test-tls-crt-new"),
		"tls.key": []byte("test-tls-key-new"),
	}

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, apis.AddToScheme(scheme))

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr, sts, internalSecret).
		WithStatusSubresource(&api.PerconaXtraDBCluster{}).
		Build()
	r := &ReconcilePerconaXtraDBCluster{
		client: cl,
		scheme: cl.Scheme(),
	}

	// No TLS rotation in progress
	inProgress, err := r.rotateSSLCertificate(ctx, cr, cr.Spec.PXC.SSLInternalSecretName)
	require.NoError(t, err)
	require.False(t, inProgress, "TLS rotation in progress")

	// Create a new TLS certificate secret
	err = cl.Create(ctx, newSecret)
	require.NoError(t, err)
	ensureSSLReconciled(t, r, cr, cr.Spec.PXC.SSLInternalSecretName)

	// Step 1: Assert that combined CA is applied
	inProgress, err = r.rotateSSLCertificate(ctx, cr, cr.Spec.PXC.SSLInternalSecretName)
	require.NoError(t, err)
	require.True(t, inProgress, "TLS rotation is not in progress")
	internalSecret = &corev1.Secret{}
	err = cl.Get(ctx, types.NamespacedName{
		Namespace: cr.GetNamespace(),
		Name:      cr.Spec.PXC.SSLInternalSecretName,
	}, internalSecret)
	require.NoError(t, err)
	currentCA := internalSecret.Data["ca.crt"]
	require.True(t, bytes.Contains(currentCA, newSecret.Data["ca.crt"]), "New CA is not appended with existing")

	// Step 2: Assert that new TLS certificate is applied
	ensureSSLReconciled(t, r, cr, cr.Spec.PXC.SSLInternalSecretName)
	inProgress, err = r.rotateSSLCertificate(ctx, cr, cr.Spec.PXC.SSLInternalSecretName)
	require.NoError(t, err)
	require.True(t, inProgress, "TLS rotation is not in progress")
	internalSecret = &corev1.Secret{}
	err = cl.Get(ctx, types.NamespacedName{
		Namespace: cr.GetNamespace(),
		Name:      cr.Spec.PXC.SSLInternalSecretName,
	}, internalSecret)
	require.NoError(t, err)

	currentTLSKey := internalSecret.Data["tls.key"]
	require.Equal(t, newSecret.Data["tls.key"], currentTLSKey, "New TLS key is not applied")
	currentTLSCert := internalSecret.Data["tls.crt"]
	require.Equal(t, newSecret.Data["tls.crt"], currentTLSCert, "New TLS certificate is not applied")

	// Step 3: old CA is replaced with new CA
	ensureSSLReconciled(t, r, cr, cr.Spec.PXC.SSLInternalSecretName)
	inProgress, err = r.rotateSSLCertificate(ctx, cr, cr.Spec.PXC.SSLInternalSecretName)
	require.NoError(t, err)
	require.True(t, inProgress, "TLS rotation is not in progress")
	internalSecret = &corev1.Secret{}
	err = cl.Get(ctx, types.NamespacedName{
		Namespace: cr.GetNamespace(),
		Name:      cr.Spec.PXC.SSLInternalSecretName,
	}, internalSecret)
	require.NoError(t, err)

	currentCA = internalSecret.Data["ca.crt"]
	require.Equal(t, newSecret.Data["ca.crt"], currentCA, "Old CA is not replaced with new CA")

	// Step 4: -new secret is deleted
	ensureSSLReconciled(t, r, cr, cr.Spec.PXC.SSLInternalSecretName)
	inProgress, err = r.rotateSSLCertificate(ctx, cr, cr.Spec.PXC.SSLInternalSecretName)
	require.NoError(t, err)
	require.False(t, inProgress, "TLS rotation is in progress")
	existing := &corev1.Secret{}
	err = cl.Get(ctx, client.ObjectKeyFromObject(newSecret), existing)
	require.Error(t, err)
	require.True(t, k8serrors.IsNotFound(err), "New secret is not deleted")
}

func ensureSSLReconciled(t *testing.T, r *ReconcilePerconaXtraDBCluster, cr *api.PerconaXtraDBCluster, secretName string) {
	sfs := statefulset.NewNode(cr).StatefulSet()
	hash, err := r.getSecretHash(cr, secretName, false)
	require.NoError(t, err)

	currentSfs := &appsv1.StatefulSet{}
	err = r.client.Get(t.Context(), client.ObjectKeyFromObject(sfs), currentSfs)
	require.NoError(t, err)

	annots := currentSfs.Spec.Template.Annotations
	if annots == nil {
		annots = make(map[string]string)
	}
	annots[sslInternalHashAnnotation] = hash
	currentSfs.Spec.Template.Annotations = annots
	err = r.client.Update(t.Context(), currentSfs)
	require.NoError(t, err)

	cr.Status.Status = api.AppStateReady
	err = r.client.Status().Update(t.Context(), cr)
	require.NoError(t, err)
}
