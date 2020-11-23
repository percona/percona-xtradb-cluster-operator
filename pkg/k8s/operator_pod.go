package k8s

import (
	"context"
	"io/ioutil"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func OperatorPod(cl client.Client) (corev1.Pod, error) {
	operatorPod := corev1.Pod{}

	nsBytes, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return operatorPod, err
	}

	ns := strings.TrimSpace(string(nsBytes))

	if err := cl.Get(context.TODO(), types.NamespacedName{
		Namespace: ns,
		Name:      os.Getenv("HOSTNAME"),
	}, &operatorPod); err != nil {
		return operatorPod, err
	}

	return operatorPod, nil
}
