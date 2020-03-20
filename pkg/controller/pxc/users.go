package pxc

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ReconcilePerconaXtraDBCluster) reconcileUsers(cr *api.PerconaXtraDBCluster) error {
	if cr.Status.Status != api.AppStateReady {
		return nil
	}

	if len(cr.Spec.Users.Secrets) == 0 {
		return nil
	}

	for _, secretName := range cr.Spec.Users.Secrets {
		err := r.handleUsersSecret(secretName, cr)
		if err != nil {
			log.Error(err, "handle users secret "+secretName)
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleUsersSecret(secretName string, cr *api.PerconaXtraDBCluster) error {
	usersSecretObj := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      secretName,
		},
		&usersSecretObj,
	)
	if err != nil && k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Wrapf(err, "get users secret '%s'", secretName)
	}

	newHash := ""
	if secretData, ok := usersSecretObj.Data["secret.yaml"]; ok {
		newHash = sha256Hash(secretData)
	}
	lastAppliedHash := ""
	if hash, ok := usersSecretObj.Annotations["last-applied"]; ok {
		lastAppliedHash = hash
		if lastAppliedHash == newHash {
			if usersSecretObj.Annotations["status"] == "succeded" || usersSecretObj.Annotations["status"] == "failed" {
				return nil
			}
		}
	}

	if len(usersSecretObj.Annotations) == 0 {
		usersSecretObj.Annotations = make(map[string]string)
	}
	usersSecretObj.Annotations["last-applied"] = newHash
	usersSecretObj.Annotations["status"] = "applying"
	err = r.client.Update(context.TODO(), &usersSecretObj)
	if err != nil {
		return errors.Wrap(err, "update secret last-applied")
	}

	operatorPod := corev1.Pod{}
	err = r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      os.Getenv("HOSTNAME"),
		},
		&operatorPod,
	)
	if err != nil {
		return errors.Wrap(err, "get operator deployment")
	}
	containerImage := operatorPod.Spec.Containers[0].Image
	imagePullSecrets := operatorPod.Spec.ImagePullSecrets
	job := users.Job(cr, secretName, newHash)
	job.Spec = users.JobSpec(secretName, containerImage, job, cr, imagePullSecrets)

	currentJob := new(batchv1.Job)
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: cr.Namespace}, currentJob)
	if err != nil && k8serrors.IsNotFound(err) {
		err = r.client.Create(context.TODO(), job)
		if err != nil {
			return errors.Wrapf(err, "create job '%s'", job.Name)
		}
		return nil
	} else if err != nil {
		return errors.Errorf("get user manager job '%s': %v", job.Name, err)
	}

	if currentJob.Labels["secret-hash"] != newHash {
		err = r.client.Delete(context.TODO(), currentJob)
		if err != nil {
			return errors.Wrap(err, "delete out of date job")
		}
		return nil
	}

	if currentJob.Status.Succeeded+currentJob.Status.Failed > 0 {
		status := ""
		if currentJob.Status.Succeeded > 0 {
			err = r.client.Delete(context.TODO(), currentJob)
			if err != nil {
				return errors.Wrap(err, "delete current job")
			}
			status = "succeded"
		}
		if currentJob.Status.Failed > 0 {
			status = "failed"
		}
		usersSecretObj.Annotations["status"] = status
		err = r.client.Update(context.TODO(), &usersSecretObj)
		if err != nil {
			return errors.Wrap(err, "update secret status")
		}
	}

	return nil
}

func sha256Hash(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
