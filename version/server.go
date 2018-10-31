package version

import (
	"encoding/json"

	"github.com/operator-framework/operator-sdk/pkg/k8sclient"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

// Server returns server version and platform (k8s|oc)
// stolen from: https://github.com/openshift/origin/blob/release-3.11/pkg/oc/cli/version/version.go#L106
func Server() (*v1alpha1.ServerVersion, error) {
	version := &v1alpha1.ServerVersion{}

	client := k8sclient.GetKubeClient().Discovery().RESTClient()

	kubeVersionBody, err := client.Get().AbsPath("/version").Do().Raw()
	switch {
	case err == nil:
		err = json.Unmarshal(kubeVersionBody, &version.Info)
		if err != nil && len(kubeVersionBody) > 0 {
			return nil, err
		}
		version.Platform = v1alpha1.PlatformKubernetes
	case kapierrors.IsNotFound(err) || kapierrors.IsUnauthorized(err) || kapierrors.IsForbidden(err):
		// this is fine! just try to get /version/openshift
	default:
		return nil, err
	}

	ocVersionBody, err := client.Get().AbsPath("/version/openshift").Do().Raw()
	switch {
	case err == nil:
		err = json.Unmarshal(ocVersionBody, &version.Info)
		if err != nil && len(ocVersionBody) > 0 {
			return nil, err
		}
		version.Platform = v1alpha1.PlatformOpenshift
	case kapierrors.IsNotFound(err) || kapierrors.IsUnauthorized(err) || kapierrors.IsForbidden(err):
	default:
		return nil, err
	}

	return version, nil
}
