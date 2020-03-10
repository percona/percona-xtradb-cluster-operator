package pxc

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"time"

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
	if len(cr.Spec.UsersSecrets) == 0 {
		return nil
	}
	secretObj := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.SecretsName,
		},
		&secretObj,
	)
	if err != nil && k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Wrapf(err, "get cluster secret '%s'", cr.Spec.SecretsName)
	}
	operator := corev1.Pod{}
	err = r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      os.Getenv("HOSTNAME"),
		},
		&operator,
	)
	if err != nil {
		return errors.Wrap(err, "get operator deployment")
	}

	for _, secretName := range cr.Spec.UsersSecrets {
		err = r.handleUsersSecret(secretName, operator.Spec.Containers[0].Image, secretObj, cr)
		if err != nil {
			log.Error(err, "handle users secret "+secretName)
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleUsersSecret(secretName, containerImage string, secretObj corev1.Secret, cr *api.PerconaXtraDBCluster) error {
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
		return errors.Wrapf(err, "get users secret '%s'", "secret-for-users")
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
	err = r.client.Update(context.TODO(), &usersSecretObj)
	if err != nil {
		return errors.Wrap(err, "update secret last-applied")
	}

	job := users.Job(cr)
	job.Spec = users.JobSpec(string(secretObj.Data["root"]), cr.Name+"-pxc", containerImage, job)

	currentJob := new(batchv1.Job)

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: cr.Namespace}, currentJob)
	if err != nil && k8serrors.IsNotFound(err) {
		err = r.client.Create(context.TODO(), job)
		if err != nil {
			return errors.Wrapf(err, "create job '%s'", job.Name)
		}
		if len(lastAppliedHash) > 0 {
			usersSecretObj.Annotations["status"] = "applying"
			err = r.client.Update(context.TODO(), &usersSecretObj)
			if err != nil {
				return errors.Wrap(err, "update secret status")
			}
		}
		time.Sleep(time.Second * 5)
		return nil
	} else if err != nil {
		return errors.Errorf("get user manager job '%s': %v", job.Name, err)
	}

	if currentJob.Status.Succeeded+currentJob.Status.Failed > 0 {
		status := ""
		if currentJob.Status.Succeeded > 0 {
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
		err = r.client.Delete(context.TODO(), currentJob)
		if err != nil {
			return errors.Wrap(err, "delete current job")
		}

		time.Sleep(time.Second * 5)
	}

	return nil
}

func sha256Hash(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
