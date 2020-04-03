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
	"gopkg.in/yaml.v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const internalPrefix = "internal-"

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

	currentJob := new(batchv1.Job)
	jobName := genName63(secretName, cr)
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: jobName, Namespace: cr.Namespace}, currentJob)
	if err != nil && k8serrors.IsNotFound(err) {
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

		err = r.InternalSecret(cr, &usersSecretObj)
		if err != nil {
			return errors.Wrap(err, "internal secret")
		}

		job := users.Job(cr, jobName, newHash)
		job.Spec = users.JobSpec(secretName, internalPrefix+cr.Spec.SecretsName, containerImage, job, cr, imagePullSecrets)

		err = r.client.Create(context.TODO(), job)
		if err != nil {
			return errors.Wrapf(err, "create job '%s'", job.Name)
		}
		return nil
	} else if err != nil {
		return errors.Errorf("get user manager job '%s': %v", jobName, err)
	}

	if currentJob.Annotations["secret-hash"] != newHash {
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
			err = r.UpdateInternalSecret(cr, &usersSecretObj)
			if err != nil {
				return errors.Wrap(err, "update internal secret")
			}
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

func (r *ReconcilePerconaXtraDBCluster) InternalSecret(cr *api.PerconaXtraDBCluster, userSecret *corev1.Secret) error {
	secretName := internalPrefix + cr.Spec.SecretsName
	secretObj := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      secretName,
		},
		&secretObj,
	)
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Wrap(err, "get internal users secret")
	} else if k8serrors.IsNotFound(err) {
		secretObj, err = r.handleUsers(cr, userSecret, false)
		if err != nil {
			return errors.Wrap(err, "handle internal secret")
		}

		err = r.client.Create(context.TODO(), &secretObj)
		if err != nil {
			return errors.Wrap(err, "create internal users secret")
		}
		return nil
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) UpdateInternalSecret(cr *api.PerconaXtraDBCluster, userSecret *corev1.Secret) error {
	secretObj, err := r.handleUsers(cr, userSecret, true)
	if err != nil {
		return errors.Wrap(err, "update internal secret")
	}
	err = r.client.Update(context.TODO(), &secretObj)
	if err != nil {
		return errors.Wrap(err, "create internal users secret")
	}
	return nil

}

func (r *ReconcilePerconaXtraDBCluster) handleUsers(cr *api.PerconaXtraDBCluster, userSecret *corev1.Secret, update bool) (corev1.Secret, error) {
	secretName := internalPrefix + cr.Spec.SecretsName
	interUsers := []users.InternalUser{}
	secretObj := corev1.Secret{}

	if update {
		err := r.client.Get(context.TODO(),
			types.NamespacedName{
				Namespace: cr.Namespace,
				Name:      secretName,
			},
			&secretObj,
		)
		if err != nil {
			return secretObj, errors.Wrap(err, "get internal users secret")
		}
		err = yaml.Unmarshal(secretObj.Data["users"], &interUsers)
		if err != nil {
			return secretObj, errors.Wrap(err, "unmarshal users secret data")
		}
		var newInterUsers []users.InternalUser
		for _, u := range interUsers {
			if u.Owner == userSecret.Name {
				continue
			}
			newInterUsers = append(newInterUsers, u)
		}
		interUsers = newInterUsers
	}

	var usersSlice users.Data
	usersData := userSecret.Data["grants.yaml"]
	err := yaml.Unmarshal(usersData, &usersSlice)
	if err != nil {
		return secretObj, errors.Wrap(err, "unmarshal users secret data")
	}

	data := make(map[string][]byte)
	for _, user := range usersSlice.Users {
		for _, host := range user.Hosts {
			var interUser users.InternalUser
			interUser.Name = user.Name + "@" + host
			interUser.Owner = userSecret.Name
			if update {
				interUser.Status = "applyed"
			} else {
				interUser.Status = "applying"
			}
			interUser.Time = time.Now().Unix()

			interUsers = append(interUsers, interUser)
		}
	}

	interUsersData, err := yaml.Marshal(interUsers)
	if err != nil {
		return secretObj, errors.Wrap(err, "marshal internal users")
	}
	data["users"] = interUsersData
	secretObj = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cr.Namespace,
		},
		Data: data,
		Type: corev1.SecretTypeOpaque,
	}

	return secretObj, nil
}

func sha256Hash(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// k8s sets the `job-name` label for the pod created by job.
// So we have to be sure that job name won't be longer than 63 symbols.
// Yet the job name has to have some meaningful name which won't be conflicting with other jobs' names.
func genName63(secretName string, cr *api.PerconaXtraDBCluster) string {
	postfix := "-pxc-usrs-mngr" + secretName

	postfixMaxLen := 36
	postfix = trimNameRight(postfix, postfixMaxLen)

	prefix := cr.Name
	prefixMaxLen := 27
	if len(prefix) > prefixMaxLen {
		prefix = prefix[:prefixMaxLen]
	}

	return prefix + postfix
}

// trimNameRight if needed cut off symbol by symbol from the name right side
// until it satisfy requirements to end with an alphanumeric character and have a length no more than ln
func trimNameRight(name string, ln int) string {
	if len(name) <= ln {
		ln = len(name)
	}

	for ; ln > 0; ln-- {
		if name[ln-1] >= 'a' && name[ln-1] <= 'z' ||
			name[ln-1] >= '0' && name[ln-1] <= '9' {
			break
		}
	}

	return name[:ln]
}
