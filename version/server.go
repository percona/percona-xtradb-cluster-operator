package version

import (
	"encoding/json"
	"net"
	"os"

	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

// Server returns server version and platform (k8s|oc)
func Server() (*api.ServerVersion, error) {
	kubeClient, _ := mustNewKubeClientAndConfig()
	client := kubeClient.Discovery().RESTClient()

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

// mustNewKubeClientAndConfig returns the in-cluster config and kubernetes client
// or if KUBERNETES_CONFIG is given an out of cluster config and client
func mustNewKubeClientAndConfig() (kubernetes.Interface, *rest.Config) {
	var cfg *rest.Config
	var err error
	if os.Getenv("KUBERNETES_CONFIG") != "" {
		cfg, err = outOfClusterConfig()
	} else {
		cfg, err = inClusterConfig()
	}
	if err != nil {
		panic(err)
	}
	return kubernetes.NewForConfigOrDie(cfg), cfg
}

// inClusterConfig returns the in-cluster config accessible inside a pod
func inClusterConfig() (*rest.Config, error) {
	// Work around https://github.com/kubernetes/kubernetes/issues/40973
	// See https://github.com/coreos/etcd-operator/issues/731#issuecomment-283804819
	if len(os.Getenv("KUBERNETES_SERVICE_HOST")) == 0 {
		addrs, err := net.LookupHost("kubernetes.default.svc")
		if err != nil {
			return nil, err
		}
		os.Setenv("KUBERNETES_SERVICE_HOST", addrs[0])
	}
	if len(os.Getenv("KUBERNETES_SERVICE_PORT")) == 0 {
		os.Setenv("KUBERNETES_SERVICE_PORT", "443")
	}
	return rest.InClusterConfig()
}

func outOfClusterConfig() (*rest.Config, error) {
	kubeconfig := os.Getenv("KUBERNETES_CONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	return config, err
}
