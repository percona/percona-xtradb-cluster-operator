package statefulset

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/test"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPXCAppContainer(t *testing.T) {
	tests := map[string]struct {
		spec              api.PXCSpec
		secrets           string
		envFromSecret     *corev1.Secret
		expectedName      string
		expectedImage     string
		expectedLDPreload string
		expectedEnvFrom   []corev1.EnvFromSource
		expectError       bool
	}{
		"success - container construction": {
			spec: api.PXCSpec{
				PodSpec: &api.PodSpec{
					Image:             "test-image",
					ImagePullPolicy:   corev1.PullIfNotPresent,
					EnvVarsSecretName: "test-secret",
				},
			},
			secrets:           "monitor-secret",
			expectedName:      "pxc",
			expectedImage:     "test-image",
			expectedLDPreload: "",
			expectedEnvFrom: []corev1.EnvFromSource{
				{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "test-secret",
						},
						Optional: ptr.To(true),
					},
				},
			},
			expectError: false,
		},
		"allocator - jemalloc": {
			spec: api.PXCSpec{
				MySQLAllocator: "jemalloc",
				PodSpec: &api.PodSpec{
					Image:             "test-image",
					ImagePullPolicy:   corev1.PullIfNotPresent,
					EnvVarsSecretName: "test-secret",
				},
			},
			secrets:           "monitor-secret",
			expectedName:      "pxc",
			expectedImage:     "test-image",
			expectedLDPreload: "/usr/lib64/libjemalloc.so.1",
			expectedEnvFrom: []corev1.EnvFromSource{
				{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "test-secret",
						},
						Optional: ptr.To(true),
					},
				},
			},
			expectError: false,
		},
		"allocator - tcmalloc": {
			spec: api.PXCSpec{
				MySQLAllocator: "tcmalloc",
				PodSpec: &api.PodSpec{
					Image:             "test-image",
					ImagePullPolicy:   corev1.PullIfNotPresent,
					EnvVarsSecretName: "test-secret",
				},
			},
			secrets:           "monitor-secret",
			expectedName:      "pxc",
			expectedImage:     "test-image",
			expectedLDPreload: "/usr/lib64/libtcmalloc.so",
			expectedEnvFrom: []corev1.EnvFromSource{
				{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "test-secret",
						},
						Optional: ptr.To(true),
					},
				},
			},
			expectError: false,
		},
		"allocator - override with envFrom": {
			spec: api.PXCSpec{
				MySQLAllocator: "jemalloc",
				PodSpec: &api.PodSpec{
					Image:             "test-image",
					ImagePullPolicy:   corev1.PullIfNotPresent,
					EnvVarsSecretName: "test-secret",
				},
			},
			secrets: "monitor-secret",
			envFromSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"LD_PRELOAD": []byte("/usr/lib64/libtcmalloc.so"),
				},
			},
			expectedName:      "pxc",
			expectedImage:     "test-image",
			expectedLDPreload: "/usr/lib64/libtcmalloc.so",
			expectedEnvFrom: []corev1.EnvFromSource{
				{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "test-secret",
						},
						Optional: ptr.To(true),
					},
				},
			},
			expectError: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cr := &api.PerconaXtraDBCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
				Spec: api.PerconaXtraDBClusterSpec{
					CRVersion: version.Version(),
					PXC:       &tt.spec,
				},
			}

			objs := []runtime.Object{cr}
			if tt.envFromSecret != nil {
				objs = append(objs, tt.envFromSecret)
			}

			client := test.BuildFakeClient(objs...)

			pxcNode := Node{cr: cr}

			c, err := pxcNode.AppContainer(t.Context(), client, tt.spec.PodSpec, tt.secrets, cr, nil)
			if tt.expectError {
				require.Error(t, err)
			}

			assert.Equal(t, tt.expectedName, c.Name)
			assert.Equal(t, tt.expectedImage, c.Image)
			assert.Equal(t, tt.expectedEnvFrom, c.EnvFrom)

			ldPreload := ""
			for _, e := range c.Env {
				if e.Name == "LD_PRELOAD" {
					ldPreload = e.Value
				}
			}
			assert.Equal(t, tt.expectedLDPreload, ldPreload)
		})
	}
}
