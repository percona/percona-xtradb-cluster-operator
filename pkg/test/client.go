package test

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake" //nolint

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

// BuildFakeClient creates a fake client to mock API calls with the mock objects
func BuildFakeClient(objs ...runtime.Object) client.Client {
	s := scheme.Scheme

	types := []runtime.Object{
		new(api.PerconaXtraDBClusterRestore),
		new(api.PerconaXtraDBClusterRestoreList),
		new(api.PerconaXtraDBClusterBackup),
		new(api.PerconaXtraDBCluster),
	}

	s.AddKnownTypes(api.SchemeGroupVersion, types...)

	toClientObj := func(objs []runtime.Object) []client.Object {
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
