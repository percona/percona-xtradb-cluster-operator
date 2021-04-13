package statefulset

import (
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

func EntrypointInitContainer(initImageName string, resources *api.PodResources, securityContext *corev1.SecurityContext, pullPolicy corev1.PullPolicy) (corev1.Container, error) {
	c := corev1.Container{
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      DataVolumeName,
				MountPath: "/var/lib/mysql",
			},
		},
		Image:           initImageName,
		ImagePullPolicy: pullPolicy,
		Name:            "pxc-init",
		Command:         []string{"/pxc-init-entrypoint.sh"},
		SecurityContext: securityContext,
	}
	res, err := app.CreateResources(resources)
	if err != nil {
		return corev1.Container{}, errors.Wrap(err, "create resources")
	}
	c.Resources = res

	return c, nil
}
