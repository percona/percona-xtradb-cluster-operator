// Package testutil holds fixtures shared by the tests of the restore
// controller and of its physical and logical restore packages.
package testutil

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake" //nolint

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
)

// deployFile returns the path of a file in the repository's deploy
// directory, wherever the test that asks for it is run from.
func deployFile(elem ...string) string {
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	return filepath.Join(append([]string{repoRoot, "deploy"}, elem...)...)
}

// ReadDefaultCR returns the cluster from deploy/cr.yaml with the given name and namespace.
func ReadDefaultCR(t *testing.T, name, namespace string) *api.PerconaXtraDBCluster {
	t.Helper()

	data, err := os.ReadFile(deployFile("cr.yaml"))
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

// ReadDefaultCRSecret returns the secret from deploy/secrets.yaml with the given name and namespace.
func ReadDefaultCRSecret(t *testing.T, name, namespace string) *corev1.Secret {
	t.Helper()

	data, err := os.ReadFile(deployFile("secrets.yaml"))
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

// ReadDefaultBackup returns the backup from deploy/backup/backup.yaml with the given name and namespace.
func ReadDefaultBackup(t *testing.T, name, namespace string) *api.PerconaXtraDBClusterBackup {
	t.Helper()

	data, err := os.ReadFile(deployFile("backup", "backup.yaml"))
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

// ReadDefaultS3Secret returns the secret from deploy/backup/backup-secret-s3.yaml with the given name and namespace.
func ReadDefaultS3Secret(t *testing.T, name, namespace string) *corev1.Secret {
	t.Helper()

	data, err := os.ReadFile(deployFile("backup", "backup-secret-s3.yaml"))
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

// ReadDefaultAzureSecret returns the secret from deploy/backup/backup-secret-azure.yaml with the given name and namespace.
func ReadDefaultAzureSecret(t *testing.T, name, namespace string) *corev1.Secret {
	t.Helper()

	data, err := os.ReadFile(deployFile("backup", "backup-secret-azure.yaml"))
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

// ReadDefaultRestore returns the restore from deploy/backup/restore.yaml with the given name and namespace.
func ReadDefaultRestore(t *testing.T, name, namespace string) *api.PerconaXtraDBClusterRestore {
	t.Helper()

	data, err := os.ReadFile(deployFile("backup", "restore.yaml"))
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

// BuildFakeClient creates a fake client to mock API calls with the mock objects
func BuildFakeClient(objs ...k8sruntime.Object) client.Client {
	s := scheme.Scheme

	types := []k8sruntime.Object{
		new(api.PerconaXtraDBClusterRestore),
		new(api.PerconaXtraDBClusterRestoreList),
		new(api.PerconaXtraDBClusterBackup),
		new(api.PerconaXtraDBCluster),
	}

	s.AddKnownTypes(api.SchemeGroupVersion, types...)

	toClientObj := func(objs []k8sruntime.Object) []client.Object {
		cliObjs := make([]client.Object, 0, len(objs))
		for _, obj := range objs {
			cliObj, ok := obj.(client.Object)
			if ok {
				cliObjs = append(cliObjs, cliObj)
			}
		}
		return cliObjs
	}

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(objs...).
		WithStatusSubresource(toClientObj(types)...).
		Build()

	return cl
}

// UpdateResource returns a copy of the object with f applied to it.
func UpdateResource[T k8sruntime.Object](res k8sruntime.Object, f func(T)) T {
	obj := res.DeepCopyObject().(T)
	f(obj)
	return obj
}

// FakeStorageClient is a storage client whose object listing is set by the test.
type FakeStorageClient struct {
	storage.Storage
	Prefix                  string
	ExpectedListObjectsPath string
	ListObjectsResponse     []string
	FailListObjects         bool
	EmptyListObjects        bool
}

func (c *FakeStorageClient) ListObjects(_ context.Context, key string) ([]string, error) {
	switch {
	case c.ExpectedListObjectsPath != "" && key != c.ExpectedListObjectsPath:
		return nil, errors.Errorf("unexpected list path: got %s want %s", key, c.ExpectedListObjectsPath)
	case c.EmptyListObjects:
		return nil, nil
	case c.FailListObjects:
		return nil, errors.New("failListObjects")
	case len(c.ListObjectsResponse) > 0:
		return c.ListObjectsResponse, nil
	}
	return []string{"some-dest/backup1", "some-dest/backup2"}, nil
}

func (c *FakeStorageClient) GetPrefix() string {
	return c.Prefix
}
