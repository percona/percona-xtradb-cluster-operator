package version

import (
	"encoding/json"

	"github.com/operator-framework/operator-sdk/pkg/k8sclient"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/rest"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

// Server returns server version and platform (k8s|oc)
func Server() (*api.ServerVersion, error) {
	client := k8sclient.GetKubeClient().Discovery().RESTClient()

	version := &api.ServerVersion{}
	var err error
	// oc 3.9
	version.Info, err = probeAPI("/version/openshift", client)
	if err == nil {
		version.Platform = api.PlatformOpenshift
		return version, nil
	}

	// oc 3.11+
	version.Info, err = probeAPI("/oapi/v1", client)
	if err == nil {
		version.Platform = api.PlatformOpenshift
		version.Info.GitVersion = "undefined (v3.11+)"
		return version, nil
	}

	// k8s
	version.Info, err = probeAPI("/version", client)
	if err == nil {
		version.Platform = api.PlatformKubernetes
		return version, nil
	}

	return version, err
}

func probeAPI(path string, client rest.Interface) (k8sversion.Info, error) {
	var vInfo k8sversion.Info
	vBody, err := client.Get().AbsPath(path).Do().Raw()
	if err != nil {
		return vInfo, err
	}

	err = json.Unmarshal(vBody, &vInfo)
	if err != nil {
		return vInfo, err
	}

	return vInfo, nil
}
