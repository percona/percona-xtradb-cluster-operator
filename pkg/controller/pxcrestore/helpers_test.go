package pxcrestore

import (
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake" //nolint

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	fakestorage "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage/fake"
)

func readDefaultCR(t *testing.T, name, namespace string) *api.PerconaXtraDBCluster {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "cr.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	cr := &api.PerconaXtraDBCluster{}

	if err := yaml.Unmarshal(data, cr); err != nil {
		t.Fatal(err)
	}

	cr.Name = name
	cr.Namespace = namespace
	cr.Spec.InitImage = "perconalab/percona-xtradb-cluster-operator:main"
	b := false
	cr.Spec.PXC.AutoRecovery = &b
	return cr
}

func readDefaultCRSecret(t *testing.T, name, namespace string) *corev1.Secret {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "secrets.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	secret := new(corev1.Secret)

	if err := yaml.Unmarshal(data, secret); err != nil {
		t.Fatal(err)
	}

	secret.Name = name
	secret.Namespace = namespace
	return secret
}

func readDefaultBackup(t *testing.T, name, namespace string) *api.PerconaXtraDBClusterBackup {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "backup", "backup.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	cr := &api.PerconaXtraDBClusterBackup{}

	if err := yaml.Unmarshal(data, cr); err != nil {
		t.Fatal(err)
	}

	cr.Name = name
	cr.Namespace = namespace
	return cr
}

func readDefaultS3Secret(t *testing.T, name, namespace string) *corev1.Secret {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "backup", "backup-secret-s3.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	secret := new(corev1.Secret)

	if err := yaml.Unmarshal(data, secret); err != nil {
		t.Fatal(err)
	}

	secret.Name = name
	secret.Namespace = namespace
	return secret
}

func readDefaultAzureSecret(t *testing.T, name, namespace string) *corev1.Secret {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "backup", "backup-secret-azure.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	secret := new(corev1.Secret)

	if err := yaml.Unmarshal(data, secret); err != nil {
		t.Fatal(err)
	}

	secret.Name = name
	secret.Namespace = namespace
	return secret
}

func readDefaultRestore(t *testing.T, name, namespace string) *api.PerconaXtraDBClusterRestore {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "backup", "restore.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	cr := &api.PerconaXtraDBClusterRestore{}

	if err := yaml.Unmarshal(data, cr); err != nil {
		t.Fatal(err)
	}

	cr.Name = name
	cr.Namespace = namespace
	return cr
}

func reconciler(cl client.Client) *ReconcilePerconaXtraDBClusterRestore {
	return &ReconcilePerconaXtraDBClusterRestore{
		client:               cl,
		scheme:               cl.Scheme(),
		newStorageClientFunc: fakestorage.NewFakeClient,
	}
}

// buildFakeClient creates a fake client to mock API calls with the mock objects
func buildFakeClient(objs ...runtime.Object) client.Client {
	s := scheme.Scheme

	s.AddKnownTypes(api.SchemeGroupVersion, new(api.PerconaXtraDBClusterRestore))
	s.AddKnownTypes(api.SchemeGroupVersion, new(api.PerconaXtraDBClusterBackup))
	s.AddKnownTypes(api.SchemeGroupVersion, new(api.PerconaXtraDBCluster))

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(objs...).
		WithStatusSubresource(&api.PerconaXtraDBClusterRestore{}).
		Build()

	return cl
}

func updateResource[T runtime.Object](res runtime.Object, f func(T)) T {
	obj := res.DeepCopyObject().(T)
	f(obj)
	return obj
}
