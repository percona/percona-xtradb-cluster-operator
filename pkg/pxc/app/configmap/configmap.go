package configmap

import (
	"errors"
	"strconv"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	corev1 "k8s.io/api/core/v1"
	res "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewConfigMap(cr *api.PerconaXtraDBCluster, cmName string) (*corev1.ConfigMap, error) {
	conf := cr.Spec.PXC.Configuration

	if len(cr.Spec.PXC.Resources.Limits.Memory) > 0 || len(cr.Spec.PXC.Resources.Requests.Memory) > 0 {
		memory := cr.Spec.PXC.Resources.Requests.Memory
		if len(cr.Spec.PXC.Resources.Limits.Memory) > 0 {
			memory = cr.Spec.PXC.Resources.Limits.Memory
		}
		autotuneParams, err := getAutoTuneParams(memory)
		if err != nil {
			return nil, err
		}
		conf += autotuneParams
	}

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cr.Namespace,
		},
		Data: map[string]string{
			"init.cnf": conf,
		},
	}

	return cm, nil
}

func getAutoTuneParams(memory string) (string, error) {
	autotuneParams := ""
	q, err := res.ParseQuantity(memory)
	if err != nil {
		return "", err
	}
	poolSize := q.Value() / int64(100) * int64(75)
	poolSizeVal := strconv.FormatInt(poolSize, 10)
	bufPool := "\ninnodb_buffer_pool_size = " + poolSizeVal
	autotuneParams += bufPool

	flushMethodVal := "O_DIRECT"
	flushMethod := "\ninnodb_flush_method = " + flushMethodVal
	autotuneParams += flushMethod

	devider := int64(12582880)
	if q.Value() < devider {
		return "", errors.New("not enough memory")
	}
	maxConnSize := q.Value() / devider
	maxConnSizeVal := strconv.FormatInt(maxConnSize, 10)
	maxSize := "\nmax_connections = " + maxConnSizeVal
	autotuneParams += maxSize

	return autotuneParams, nil
}
