package version

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/rest"

	"github.com/percona/percona-xtradb-cluster-operator/clientcmd"
)

var (
	cVersion *ServerVersion
	mx       sync.Mutex
)

const (
	PlatformUndef      Platform = ""
	PlatformKubernetes Platform = "kubernetes"
	PlatformOpenshift  Platform = "openshift"
)

type Platform string

// ServerVersion represents info about k8s / openshift server version
type ServerVersion struct {
	Platform Platform
	Info     k8sversion.Info
}

// Server returns server version and platform (k8s|oc)
// it performs API requests for the first invocation and then returns "cached" value
func Server() (*ServerVersion, error) {
	mx.Lock()
	defer mx.Unlock()
	if cVersion != nil {
		return cVersion, nil
	}

	v, err := GetServer()
	if err != nil {
		return nil, err
	}

	cVersion = v

	return cVersion, nil
}

// GetServer make request to platform server and returns server version and platform (k8s|oc)
func GetServer() (*ServerVersion, error) {
	cl, err := clientcmd.NewClient()
	if err != nil {
		return nil, fmt.Errorf("create REST client: %v", err)
	}
	client := cl.REST()

	version := &ServerVersion{}
	// oc 3.9
	version.Info, err = probeAPI("/version/openshift", client)
	if err == nil {
		version.Platform = PlatformOpenshift
		return version, nil
	}

	// oc 3.11+
	version.Info, err = probeAPI("/oapi/v1", client)
	if err == nil {
		version.Platform = PlatformOpenshift
		version.Info.GitVersion = "undefined (v3.11+)"
		return version, nil
	}

	// openshift 4.0
	version.Info, err = probeAPI("/apis/quota.openshift.io", client)
	if err == nil {
		version.Platform = PlatformOpenshift
		version.Info.GitVersion = "undefined (v4.0+)"
		return version, nil
	}

	// k8s
	version.Info, err = probeAPI("/version", client)
	if err == nil {
		version.Platform = PlatformKubernetes
		return version, nil
	}

	return version, err
}

func probeAPI(path string, client rest.Interface) (k8sversion.Info, error) {
	var vInfo k8sversion.Info
	vBody, err := client.Get().AbsPath(path).Do(context.TODO()).Raw()
	if err != nil {
		return vInfo, err
	}

	err = json.Unmarshal(vBody, &vInfo)
	if err != nil {
		return vInfo, err
	}

	return vInfo, nil
}
