package configmap

import (
	"fmt"
	"strconv"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	corev1 "k8s.io/api/core/v1"
	res "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewConfigMap(cr *api.PerconaXtraDBCluster, cmName string) *corev1.ConfigMap {
	autotune := ""
	if len(cr.Spec.PXC.Resources.Requests.Memory) > 0 && len(cr.Spec.PXC.Resources.Requests.CPU) > 0 {
		q, err := res.ParseQuantity(cr.Spec.PXC.Resources.Limits.Memory)
		if err != nil {
			fmt.Println("error:", err)
		}
		poolSize := q.Value() / int64(100) * int64(75)
		strPoolSize := strconv.FormatInt(poolSize, 10)
		bufPool := "\ninnodb_buffer_pool_size = " + strPoolSize
		autotune += bufPool

		flushMethod := "\ninnodb_flush_method = O_DIRECT"
		autotune += flushMethod

		devider := int64(12582880)
		maxConnSize := q.Value() / devider
		strMaxConnSize := strconv.FormatInt(maxConnSize, 10)
		maxSize := "\nmax_connections = " + strMaxConnSize
		autotune += maxSize
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
			"init.cnf": cr.Spec.PXC.Configuration,
		},
	}

	return cm
}
