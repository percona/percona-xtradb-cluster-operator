package binlogcollector

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
)

func TestGetDeployment(t *testing.T) {
	createCR := func() *api.PerconaXtraDBCluster {
		return &api.PerconaXtraDBCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: api.PerconaXtraDBClusterSpec{
				CRVersion: version.Version(),
				PXC: &api.PXCSpec{
					PodSpec: &api.PodSpec{
						Enabled:                  true,
						Size:                     3,
						ContainerSecurityContext: &corev1.SecurityContext{},
					},
				},
				Backup: &api.BackupSpec{
					PITR: api.PITRSpec{
						Enabled:            true,
						StorageName:        "test-storage",
						TimeBetweenUploads: 60.0,
						TimeoutSeconds:     30.0,
					},
					Image:           "percona/pxc-backup:1.0",
					ImagePullPolicy: corev1.PullAlways,
					Storages: map[string]*api.BackupStorageSpec{
						"test-storage": {
							Type: "s3",
							S3: &api.BackupStorageS3Spec{
								Bucket:            "test-bucket",
								Region:            "us-east-1",
								CredentialsSecret: "minio-secret",
								EndpointURL:       "https://minio-service.default:9000/",
							},
							Labels: map[string]string{
								"storage-type": "s3",
							},
							Annotations: map[string]string{
								"annotation-key": "annotation-value",
							},
							ContainerSecurityContext: &corev1.SecurityContext{},
							PodSecurityContext:       &corev1.PodSecurityContext{},
						},
					},
				},
			},
		}
	}

	tests := []struct {
		name                string
		cr                  *api.PerconaXtraDBCluster
		existingMatchLabels map[string]string
		expectedLabels      map[string]string
		expectedMatchLabels map[string]string
	}{
		{
			name:                "Default labels and matchLabels",
			cr:                  createCR(),
			existingMatchLabels: nil,
			expectedLabels: map[string]string{
				"app.kubernetes.io/component":  "pitr",
				"app.kubernetes.io/instance":   "test-cluster",
				"app.kubernetes.io/managed-by": "percona-xtradb-cluster-operator",
				"app.kubernetes.io/name":       "percona-xtradb-cluster",
				"app.kubernetes.io/part-of":    "percona-xtradb-cluster",
				"storage-type":                 "s3",
			},
			expectedMatchLabels: map[string]string{
				"app.kubernetes.io/component":  "pitr",
				"app.kubernetes.io/instance":   "test-cluster",
				"app.kubernetes.io/managed-by": "percona-xtradb-cluster-operator",
				"app.kubernetes.io/name":       "percona-xtradb-cluster",
				"app.kubernetes.io/part-of":    "percona-xtradb-cluster",
			},
		},
		{
			name: "Custom existing matchLabels",
			cr:   createCR(),
			existingMatchLabels: map[string]string{
				"custom-label": "custom-value",
			},
			expectedLabels: map[string]string{
				"app.kubernetes.io/component":  "pitr",
				"app.kubernetes.io/instance":   "test-cluster",
				"app.kubernetes.io/managed-by": "percona-xtradb-cluster-operator",
				"app.kubernetes.io/name":       "percona-xtradb-cluster",
				"app.kubernetes.io/part-of":    "percona-xtradb-cluster",
				"storage-type":                 "s3",
			},
			expectedMatchLabels: map[string]string{
				"custom-label": "custom-value",
			},
		},
		{
			name: "Version 1.16.0 includes labels on deployment",
			cr: func() *api.PerconaXtraDBCluster {
				cr := createCR()
				cr.Spec.CRVersion = "1.16.0"
				return cr
			}(),
			existingMatchLabels: nil,
			expectedLabels: map[string]string{
				"app.kubernetes.io/component":  "pitr",
				"app.kubernetes.io/instance":   "test-cluster",
				"app.kubernetes.io/managed-by": "percona-xtradb-cluster-operator",
				"app.kubernetes.io/name":       "percona-xtradb-cluster",
				"app.kubernetes.io/part-of":    "percona-xtradb-cluster",
				"storage-type":                 "s3",
			},
			expectedMatchLabels: map[string]string{
				"app.kubernetes.io/component":  "pitr",
				"app.kubernetes.io/instance":   "test-cluster",
				"app.kubernetes.io/managed-by": "percona-xtradb-cluster-operator",
				"app.kubernetes.io/name":       "percona-xtradb-cluster",
				"app.kubernetes.io/part-of":    "percona-xtradb-cluster",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			depl, err := GetDeployment(tt.cr, "perconalab/percona-xtradb-cluster-operator:main", tt.existingMatchLabels)
			if err != nil {
				t.Errorf("GetDeployment() error = %v", err)
				return
			}

			if !reflect.DeepEqual(depl.Spec.Template.ObjectMeta.Labels, tt.expectedLabels) {
				t.Errorf("Pod template labels = %v, want %v", depl.Spec.Template.ObjectMeta.Labels, tt.expectedLabels)
			}

			if !reflect.DeepEqual(depl.Spec.Selector.MatchLabels, tt.expectedMatchLabels) {
				t.Errorf("Selector matchLabels = %v, want %v", depl.Spec.Selector.MatchLabels, tt.expectedMatchLabels)
			}

			if tt.cr.CompareVersionWith("1.16.0") >= 0 && tt.cr.CompareVersionWith("1.18.0") < 0 {
				if !reflect.DeepEqual(depl.Labels, tt.expectedLabels) {
					t.Errorf("Deployment labels = %v, want %v", depl.Labels, tt.expectedLabels)
				}
			} else {
				if len(depl.Labels) != 0 {
					t.Errorf("Deployment labels = %v, want empty", depl.Labels)
				}
			}
		})
	}
}
