package k8s

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
)

const WatchNamespaceEnvVar = "WATCH_NAMESPACE"

// GetWatchNamespace returns the namespace the operator should be watching for changes
func GetWatchNamespace() (string, error) {
	ns, found := os.LookupEnv(WatchNamespaceEnvVar)
	if !found {
		return "", fmt.Errorf("%s must be set", WatchNamespaceEnvVar)
	}
	return ns, nil
}

// GetOperatorNamespace returns the namespace of the operator pod
func GetOperatorNamespace() (string, error) {
	nsBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(nsBytes)), nil
}

func GetInitImage(ctx context.Context, cr *api.PerconaXtraDBCluster, cli client.Client) (string, error) {
	if len(cr.Spec.InitContainer.Image) > 0 {
		return cr.Spec.InitContainer.Image, nil
	}
	if len(cr.Spec.InitImage) > 0 {
		return cr.Spec.InitImage, nil
	}
	operatorPod, err := OperatorPod(ctx, cli)
	if err != nil {
		return "", errors.Wrap(err, "get operator deployment")
	}
	imageName, err := operatorImageName(&operatorPod)
	if err != nil {
		return "", err
	}
	if cr.CompareVersionWith(version.Version()) != 0 {
		imageName = strings.Split(imageName, ":")[0] + ":" + cr.Spec.CRVersion
	}
	return imageName, nil
}

func IsPodReady(pod corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		if condition.Type == corev1.PodReady {
			return true
		}
	}
	return false
}

func operatorImageName(operatorPod *corev1.Pod) (string, error) {
	for _, c := range operatorPod.Spec.Containers {
		if c.Name == "percona-xtradb-cluster-operator" {
			return c.Image, nil
		}
	}
	return "", errors.New("operator image not found")
}
