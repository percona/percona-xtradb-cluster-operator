package pxc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/apis"
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
)

func TestReconcilePersistentVolumes(t *testing.T) {
	tests := []struct {
		name                string
		requested           string
		configured          string
		actual              string
		resizeInProgress    bool
		volumeExpansion     bool
		expectSTSDeleted    bool
		expectResizeCleared bool
		expectErrContains   string
		expectCRStorage     string
	}{
		{
			name:                "finishes resize when pvc exceeds requested size",
			requested:           "1200Mi",
			configured:          "1200Mi",
			actual:              "6G",
			resizeInProgress:    true,
			expectSTSDeleted:    true,
			expectResizeCleared: true,
		},
		{
			name:             "deletes statefulset when requested matches actual but template differs",
			requested:        "1200Mi",
			configured:       "6G",
			actual:           "1200Mi",
			expectSTSDeleted: true,
		},
		{
			name:             "deletes statefulset when actual exceeds requested and template is lower",
			requested:        "1200Mi",
			configured:       "1Gi",
			actual:           "6G",
			expectSTSDeleted: true,
		},
		{
			name:       "does nothing when requested configured and actual sizes are aligned",
			requested:  "1200Mi",
			configured: "1200Mi",
			actual:     "1200Mi",
		},
		{
			name:       "does nothing when requested and configured sizes are aligned",
			requested:  "1200Mi",
			configured: "1200Mi",
			actual:     "6G",
		},
		{
			name:              "rejects shrink when configured is higher than requested and actual is higher than requested",
			requested:         "1200Mi",
			configured:        "2Gi",
			actual:            "6G",
			volumeExpansion:   true,
			expectErrContains: "requested storage (1200Mi) is less than actual storage (6G)",
			expectCRStorage:   "2Gi",
		},
		{
			name:             "deletes statefulset when pvc already matches increased request but template is stale",
			requested:        "2Gi",
			configured:       "1200Mi",
			actual:           "2Gi",
			expectSTSDeleted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requested := resource.MustParse(tt.requested)
			configured := resource.MustParse(tt.configured)
			actual := resource.MustParse(tt.actual)

			cr, err := readDefaultCR("some-cluster", "ns")
			require.NoError(t, err)

			cr.Spec.PXC.Size = 1
			cr.Spec.PXC.VolumeSpec.PersistentVolumeClaim.Resources.Requests = corev1.ResourceList{
				corev1.ResourceStorage: requested,
			}
			cr.Spec.VolumeExpansionEnabled = tt.volumeExpansion
			if tt.resizeInProgress {
				cr.Annotations = map[string]string{
					pxcv1.AnnotationPVCResizeInProgress: time.Now().Add(-time.Minute).Format(time.RFC3339),
				}
			}

			sts := statefulset.NewNode(cr).StatefulSet()
			sts.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "datadir",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: configured,
							},
						},
					},
				},
			}

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sts.Name + "-0",
					Namespace: cr.Namespace,
					Labels:    naming.LabelsPXC(cr),
				},
			}
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "datadir-" + sts.Name + "-0",
					Namespace: cr.Namespace,
					Labels:    naming.LabelsPXC(cr),
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: actual,
						},
					},
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Capacity: corev1.ResourceList{
						corev1.ResourceStorage: actual,
					},
				},
			}

			s := runtime.NewScheme()
			require.NoError(t, clientgoscheme.AddToScheme(s))
			require.NoError(t, apis.AddToScheme(s))

			r := &ReconcilePerconaXtraDBCluster{
				client: fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(cr, sts, pod, pvc).Build(),
				scheme: s,
			}

			err = r.reconcilePersistentVolumes(t.Context(), cr)
			if tt.expectErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErrContains)
			} else {
				require.NoError(t, err)
			}

			fetchedSTS := &appsv1.StatefulSet{}
			err = r.client.Get(t.Context(), client.ObjectKeyFromObject(sts), fetchedSTS)
			if tt.expectSTSDeleted {
				assert.Error(t, err)
				assert.True(t, client.IgnoreNotFound(err) == nil)
			} else {
				assert.NoError(t, err)
			}

			fetchedCR := &pxcv1.PerconaXtraDBCluster{}
			err = r.client.Get(t.Context(), client.ObjectKeyFromObject(cr), fetchedCR)
			require.NoError(t, err)
			if tt.expectResizeCleared {
				assert.NotContains(t, fetchedCR.GetAnnotations(), pxcv1.AnnotationPVCResizeInProgress)
			}
			if tt.expectCRStorage != "" {
				expected := resource.MustParse(tt.expectCRStorage)
				assert.Equal(t, expected, fetchedCR.Spec.PXC.VolumeSpec.PersistentVolumeClaim.Resources.Requests[corev1.ResourceStorage])
			}
		})
	}
}

func TestReconcilePersistentVolumesVolumeExternalAutoscaling(t *testing.T) {
	const (
		namespace      = "test-ns"
		clusterName    = "test-cluster"
		configuredSize = "1Gi"
		requestedSize  = "2Gi"
	)

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, apis.AddToScheme(scheme))

	labels := naming.LabelsPXC(&pxcv1.PerconaXtraDBCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
		},
	})

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName + "-pxc",
			Namespace: namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "datadir",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(configuredSize),
							},
						},
					},
				},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-pxc-0",
			Namespace: namespace,
			Labels:    labels,
		},
	}

	cr := &pxcv1.PerconaXtraDBCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
		},
		Spec: pxcv1.PerconaXtraDBClusterSpec{
			PXC: &pxcv1.PXCSpec{
				PodSpec: &pxcv1.PodSpec{
					Size: 1,
					VolumeSpec: &pxcv1.VolumeSpec{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimSpec{
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse(requestedSize),
								},
							},
						},
					},
				},
			},
		},
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadir-test-cluster-pxc-0",
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(configuredSize),
				},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(configuredSize),
			},
		},
	}

	tests := map[string]struct {
		volumeExternalAutoscaling bool
		volumeExpansionEnabled    bool
		expectRequestedSize       string
		expectPVCSize             string
		expectResizeAnnotation    bool
	}{
		"external autoscaling enabled, volume expansion enabled": {
			volumeExternalAutoscaling: true,
			volumeExpansionEnabled:    true,
			expectRequestedSize:       requestedSize,
			expectPVCSize:             configuredSize,
			expectResizeAnnotation:    false,
		},
		"external autoscaling enabled, volume expansion disabled": {
			volumeExternalAutoscaling: true,
			volumeExpansionEnabled:    false,
			expectRequestedSize:       requestedSize,
			expectPVCSize:             configuredSize,
			expectResizeAnnotation:    false,
		},
		"external autoscaling disabled, volume expansion enabled": {
			volumeExternalAutoscaling: false,
			volumeExpansionEnabled:    true,
			expectRequestedSize:       requestedSize,
			expectPVCSize:             requestedSize,
			expectResizeAnnotation:    true,
		},
		"external autoscaling disabled, volume expansion disabled": {
			volumeExternalAutoscaling: false,
			volumeExpansionEnabled:    false,
			expectRequestedSize:       configuredSize,
			expectPVCSize:             configuredSize,
			expectResizeAnnotation:    false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()

			cr := cr.DeepCopy()
			pvc := pvc.DeepCopy()

			cr.Spec.StorageScaling = &pxcv1.StorageScalingSpec{
				VolumeExternalAutoscaling: tt.volumeExternalAutoscaling,
				EnableVolumeScaling:       tt.volumeExpansionEnabled,
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(cr, sts, pvc, pod).
				Build()

			r := &ReconcilePerconaXtraDBCluster{
				client:  cl,
				scheme:  scheme,
				lockers: newLockStore(),
			}

			err := r.reconcilePersistentVolumes(ctx, cr)
			require.NoError(t, err)

			gotCR := new(pxcv1.PerconaXtraDBCluster)
			require.NoError(t, cl.Get(ctx, client.ObjectKeyFromObject(cr), gotCR))
			assert.Equal(t, tt.expectResizeAnnotation, gotCR.PVCResizeInProgress())

			requestedSize := cr.Spec.PXC.VolumeSpec.PersistentVolumeClaim.Resources.Requests[corev1.ResourceStorage]
			assert.Zero(
				t,
				requestedSize.Cmp(resource.MustParse(tt.expectRequestedSize)),
			)

			gotPVC := new(corev1.PersistentVolumeClaim)
			require.NoError(t, cl.Get(ctx, client.ObjectKeyFromObject(pvc), gotPVC))
			pvcSize := gotPVC.Spec.Resources.Requests[corev1.ResourceStorage]
			assert.Zero(
				t,
				pvcSize.Cmp(resource.MustParse(tt.expectPVCSize)),
			)
		})
	}
}
