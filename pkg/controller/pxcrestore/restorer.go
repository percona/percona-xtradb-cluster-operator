package pxcrestore

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
)

var (
	errWaitValidate = errors.New("wait for validation")
	errWaitInit     = errors.New("wait for init")
)

type Restorer interface {
	Init(ctx context.Context) error
	Job(context.Context) (*batchv1.Job, error)
	PITRJob(context.Context) (*batchv1.Job, error)
	Finalize(ctx context.Context) error
	Validate(ctx context.Context) error
	ValidateJob(ctx context.Context, job *batchv1.Job) error
}

type s3 struct{ *restorerOptions }

func (s *s3) Init(context.Context) error { return nil }

func (s *s3) Finalize(context.Context) error { return nil }

func (s *s3) Job(ctx context.Context) (*batchv1.Job, error) {
	return backup.RestoreJob(ctx, s.cr, s.bcp, s.cluster, s.initImage, s.scheme, s.bcp.Status.Destination, false)
}

func (s *s3) PITRJob(ctx context.Context) (*batchv1.Job, error) {
	return backup.RestoreJob(ctx, s.cr, s.bcp, s.cluster, s.initImage, s.scheme, s.bcp.Status.Destination, true)
}

func (s *s3) ValidateJob(ctx context.Context, job *batchv1.Job) error {
	if s.bcp.Status.S3.CredentialsSecret == "" {
		// Skip validation if the credentials secret isn't set.
		// This allows authentication via IAM roles.
		// More info: https://github.com/percona/k8spxc-docs/blob/87f98e6ddae8114474836c0610155d05d3531e03/docs/backups-storage.md?plain=1#L116-L126
		return nil
	}

	return s.restorerOptions.ValidateJob(ctx, job)
}

func (s *s3) Validate(ctx context.Context) error {
	opts, err := storage.GetOptionsFromBackup(ctx, s.k8sClient, s.cluster, s.bcp)
	if err != nil {
		return errors.Wrap(err, "failed to get storage options")
	}
	s3cli, err := s.newStorageClient(ctx, opts)
	if err != nil {
		return errors.Wrap(err, "failed to create s3 client")
	}

	backupPath := backupListPath(s3cli.GetPrefix(), s.bcp.Status.Destination)
	objs, err := s3cli.ListObjects(ctx, backupPath)
	if err != nil {
		return errors.Wrap(err, "failed to list objects")
	}
	if len(objs) == 0 {
		return errors.New("backup not found")
	}

	if err := validatePITRTarget(s.bcp, s.cr); err != nil {
		return errors.Wrap(err, "invalid pitr target")
	}
	return nil
}

type pvc struct{ *restorerOptions }

func (s *pvc) Validate(ctx context.Context) error {
	destination := s.bcp.Status.Destination

	pod, err := backup.PVCRestorePod(s.cr, s.bcp.Status.StorageName, destination.BackupName(), s.cluster, s.initImage)
	if err != nil {
		return errors.Wrap(err, "restore pod")
	}
	if err := controllerutil.SetControllerReference(s.cr, pod, s.scheme); err != nil {
		return err
	}
	pod.Name += "-verify"
	pod.Spec.Containers[0].Command = []string{"bash", "-c", `[[ $(stat -c%s /backup/xtrabackup.stream) -gt 5000000 ]]`}
	pod.Spec.RestartPolicy = corev1.RestartPolicyNever

	err = s.k8sClient.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, pod)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			if err := s.k8sClient.Create(ctx, pod); err != nil {
				return errors.Wrap(err, "failed to create pod")
			}
			return errWaitValidate
		}
		return errors.Wrap(err, "get pod status")
	}

	switch pod.Status.Phase {
	case corev1.PodFailed:
		return errors.Errorf("backup files not found on %s", destination)
	case corev1.PodSucceeded:
	default:
		return errWaitValidate
	}

	if err := validatePITRTarget(s.bcp, s.cr); err != nil {
		return errors.Wrap(err, "invalid pitr target")
	}

	return nil
}

func (s *pvc) Job(ctx context.Context) (*batchv1.Job, error) {
	return backup.RestoreJob(ctx, s.cr, s.bcp, s.cluster, s.initImage, s.scheme, "", false)
}

func (s *pvc) PITRJob(ctx context.Context) (*batchv1.Job, error) {
	return nil, errors.New("pitr restore is not supported for pvc")
}

func (s *pvc) Init(ctx context.Context) error {
	destination := s.bcp.Status.Destination

	svc := backup.PVCRestoreService(s.cr, s.cluster)
	if err := controllerutil.SetControllerReference(s.cr, svc, s.scheme); err != nil {
		return err
	}
	pod, err := backup.PVCRestorePod(s.cr, s.bcp.Status.StorageName, destination.BackupName(), s.cluster, s.initImage)
	if err != nil {
		return errors.Wrap(err, "restore pod")
	}
	if err := controllerutil.SetControllerReference(s.cr, pod, s.scheme); err != nil {
		return err
	}

	initInProcess := true

	if err := s.k8sClient.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, svc); k8serrors.IsNotFound(err) {
		initInProcess = false
	}

	if err := s.k8sClient.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: svc.Namespace}, pod); k8serrors.IsNotFound(err) {
		initInProcess = false
	}

	if !initInProcess {
		if err := s.k8sClient.Delete(ctx, svc); client.IgnoreNotFound(err) != nil {
			return err
		}
		if err := s.k8sClient.Delete(ctx, pod); client.IgnoreNotFound(err) != nil {
			return err
		}

		err = s.k8sClient.Create(ctx, svc)
		if client.IgnoreAlreadyExists(err) != nil {
			return errors.Wrap(err, "create service")
		}
		err = s.k8sClient.Create(ctx, pod)
		if client.IgnoreAlreadyExists(err) != nil {
			return errors.Wrap(err, "create pod")
		}
	}

	if pod.Status.Phase != corev1.PodRunning {
		return errWaitInit
	}
	return nil
}

func (s *pvc) Finalize(ctx context.Context) error {
	svc := backup.PVCRestoreService(s.cr, s.cluster)
	if err := s.k8sClient.Delete(ctx, svc); client.IgnoreNotFound(err) != nil {
		return errors.Wrap(err, "failed to delete pvc service")
	}
	pod, err := backup.PVCRestorePod(s.cr, s.bcp.Status.StorageName, s.bcp.Status.Destination.BackupName(), s.cluster, s.initImage)
	if err != nil {
		return err
	}
	if err := s.k8sClient.Delete(ctx, pod); client.IgnoreNotFound(err) != nil {
		return errors.Wrap(err, "failed to delete pvc pod")
	}
	pod.Name += "-verify"
	if err := s.k8sClient.Delete(ctx, pod); client.IgnoreNotFound(err) != nil {
		return errors.Wrap(err, "failed to delete pvc pod")
	}
	return nil
}

type azure struct{ *restorerOptions }

func (s *azure) Init(context.Context) error { return nil }

func (s *azure) Finalize(context.Context) error { return nil }

func (s *azure) Job(ctx context.Context) (*batchv1.Job, error) {
	return backup.RestoreJob(ctx, s.cr, s.bcp, s.cluster, s.initImage, s.scheme, s.bcp.Status.Destination, false)
}

func (s *azure) PITRJob(ctx context.Context) (*batchv1.Job, error) {
	return backup.RestoreJob(ctx, s.cr, s.bcp, s.cluster, s.initImage, s.scheme, s.bcp.Status.Destination, true)
}

func (s *azure) Validate(ctx context.Context) error {
	opts, err := storage.GetOptionsFromBackup(ctx, s.k8sClient, s.cluster, s.bcp)
	if err != nil {
		return errors.Wrap(err, "failed to get storage options")
	}
	azurecli, err := s.newStorageClient(ctx, opts)
	if err != nil {
		return errors.Wrap(err, "failed to create s3 client")
	}

	backupPath := backupListPath(azurecli.GetPrefix(), s.bcp.Status.Destination)
	blobs, err := azurecli.ListObjects(ctx, backupPath)
	if err != nil {
		return errors.Wrap(err, "list blobs")
	}

	if len(blobs) == 0 {
		return errors.New("no backups found")
	}

	if err := validatePITRTarget(s.bcp, s.cr); err != nil {
		return errors.Wrap(err, "invalid pitr target")
	}
	return nil
}

func backupListPath(storagePrefix string, destination api.PXCBackupDestination) string {
	backupPath := destination.PathWithoutBucket()
	storagePrefix = strings.TrimSuffix(storagePrefix, "/")
	if storagePrefix != "" {
		backupPath = strings.TrimPrefix(backupPath, storagePrefix+"/")
	}
	return strings.TrimSuffix(backupPath, "/") + "/"
}

func (r *ReconcilePerconaXtraDBClusterRestore) getRestorer(
	ctx context.Context,
	cr *api.PerconaXtraDBClusterRestore,
	bcp *api.PerconaXtraDBClusterBackup,
	cluster *api.PerconaXtraDBCluster,
) (Restorer, error) {
	s := restorerOptions{
		cr:               cr,
		bcp:              bcp,
		cluster:          cluster,
		k8sClient:        r.client,
		scheme:           r.scheme,
		newStorageClient: r.newStorageClientFunc,
	}
	initImage, err := k8s.GetInitImage(ctx, cluster, r.client)
	if err != nil {
		return nil, errors.New("failed to get init image")
	}
	s.initImage = initImage

	switch s.bcp.Status.Destination.StorageTypePrefix() {
	case api.PVCStoragePrefix:
		sr := pvc{&s}
		return &sr, nil
	case api.AwsBlobStoragePrefix:
		sr := s3{&s}
		return &sr, nil
	case api.AzureBlobStoragePrefix:
		sr := azure{&s}
		return &sr, nil
	}
	return nil, errors.Errorf("unknown backup storage type")
}

type restorerOptions struct {
	cr               *api.PerconaXtraDBClusterRestore
	bcp              *api.PerconaXtraDBClusterBackup
	cluster          *api.PerconaXtraDBCluster
	k8sClient        client.Client
	scheme           *runtime.Scheme
	newStorageClient storage.NewClientFunc
	initImage        string
}

func (opts *restorerOptions) ValidateJob(ctx context.Context, job *batchv1.Job) error {
	cl := opts.k8sClient

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

func validatePITRTarget(
	backup *api.PerconaXtraDBClusterBackup,
	restore *api.PerconaXtraDBClusterRestore,
) error {
	if restore.Spec.PITR == nil {
		return nil
	}

	switch restore.Spec.PITR.Type {
	case api.PITRTypeDate:
		return validatePITRDatetime(backup, restore.Spec.PITR.Date)
	}
	return nil
}

func validatePITRDatetime(
	backup *api.PerconaXtraDBClusterBackup,
	target string,
) error {
	targetTime, err := time.Parse(time.DateTime, target)
	if err != nil {
		return errors.Wrap(err, "parse datetime")
	}

	if backup.Status.LatestRestorableTime.IsZero() {
		return errors.New("latest restorable time is not known")
	}

	if backup.Status.LatestRestorableTime.Time.Before(targetTime) {
		return errors.New("target datetime is after the latest restorable time")
	}
	return nil
}
