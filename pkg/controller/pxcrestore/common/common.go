// Package common holds what the physical and the logical restore controllers
// share: the dependencies they are built from and a few helpers.
package common

import (
	"context"
	"sort"
	"strings"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/percona/percona-xtradb-cluster-operator/clientcmd"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
)

// Options is what a restore controller needs to move one restore forward.
type Options struct {
	Client           client.Client
	ClientCmd        clientcmd.Client
	Scheme           *runtime.Scheme
	NewStorageClient storage.NewClientFunc
	CR               *api.PerconaXtraDBClusterRestore
	Cluster          *api.PerconaXtraDBCluster
	Backup           *api.PerconaXtraDBClusterBackup
}

// FailRestore marks the restore as failed with the given reason.
func FailRestore(cr *api.PerconaXtraDBClusterRestore, reason string) {
	cr.Status.State = api.RestoreFailed
	cr.Status.Comments = reason
}

// IsJobFinished reports whether the job has completed. It returns an error
// when the job has failed.
func IsJobFinished(checkJob *batchv1.Job) (bool, error) {
	for _, c := range checkJob.Status.Conditions {
		if c.Status != corev1.ConditionTrue {
			continue
		}

		switch c.Type {
		case batchv1.JobComplete:
			return true, nil
		case batchv1.JobFailed:
			return false, errors.Errorf("job %s failed: %s", checkJob.Name, c.Message)
		}
	}
	return false, nil
}

// BackupListPath returns the path, relative to the storage prefix, under
// which the objects of the backup are listed.
func BackupListPath(storagePrefix string, destination api.PXCBackupDestination) string {
	backupPath := destination.PathWithoutBucket()
	storagePrefix = strings.TrimSuffix(storagePrefix, "/")
	if storagePrefix != "" {
		backupPath = strings.TrimPrefix(backupPath, storagePrefix+"/")
	}
	return strings.TrimSuffix(backupPath, "/") + "/"
}

// ValidateJobSecrets checks that every secret the job's containers take
// environment variables from exists.
func ValidateJobSecrets(ctx context.Context, cl client.Client, job *batchv1.Job) error {
	secrets := []string{}
	for _, container := range job.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil && env.ValueFrom.SecretKeyRef.Name != "" {
				secrets = append(secrets, env.ValueFrom.SecretKeyRef.Name)
			}
		}
	}

	notExistingSecrets := make(map[string]struct{})
	for _, secret := range secrets {
		err := cl.Get(ctx, types.NamespacedName{
			Name:      secret,
			Namespace: job.Namespace,
		}, new(corev1.Secret))
		if err != nil {
			if k8serrors.IsNotFound(err) {
				notExistingSecrets[secret] = struct{}{}
				continue
			}
			return err
		}
	}
	if len(notExistingSecrets) > 0 {
		secrets := make([]string, 0, len(notExistingSecrets))
		for k := range notExistingSecrets {
			secrets = append(secrets, k)
		}
		sort.StringSlice(secrets).Sort()
		return errors.Errorf("secrets %s not found", strings.Join(secrets, ", "))
	}

	return nil
}
