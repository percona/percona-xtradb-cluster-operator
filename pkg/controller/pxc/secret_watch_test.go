package pxc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

func TestPXCReferencingSecret(t *testing.T) {
	cluster := func(namespace, name, secretsName string) *api.PerconaXtraDBCluster {
		return &api.PerconaXtraDBCluster{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
			Spec:       api.PerconaXtraDBClusterSpec{SecretsName: secretsName},
		}
	}
	s := runtime.NewScheme()
	require.NoError(t, api.SchemeBuilder.AddToScheme(s))
	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithIndex(&api.PerconaXtraDBCluster{}, secretsNameField, indexSecretsName).
		WithObjects(
			cluster("pxc", "cluster1", "cluster1-secrets"),
			cluster("pxc", "cluster2", "other-secrets"),
			cluster("other", "cluster1", "cluster1-secrets"),
		).
		Build()

	// The Secret watch is a metadata watch: the handler gets Secret metadata.
	secret := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Namespace: "pxc", Name: "cluster1-secrets"}}
	secret.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))

	assert.Equal(t,
		[]reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: "pxc", Name: "cluster1"}}},
		pxcReferencingSecret(cl)(t.Context(), secret))
}
