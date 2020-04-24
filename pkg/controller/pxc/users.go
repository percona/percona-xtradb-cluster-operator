package pxc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"time"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const internalPrefix = "internal-"

const (
	statusFailed    = "failed"
	statusSucceeded = "succeeded"
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

	dataChanged := usersDataChanged(&newHash, &usersSecretObj)
	if !dataChanged {
		if usersSecretObj.Annotations["status"] == statusSucceeded || usersSecretObj.Annotations["status"] == statusFailed {
			return nil
		}
	}

	if len(usersSecretObj.Annotations) == 0 {
		usersSecretObj.Annotations = make(map[string]string)
	}

	internalSecret, err := r.getInternalSecret(cr, &usersSecretObj)
	if err != nil {
		return errors.Wrap(err, "internal secret")
	}

	if dataChanged {
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

		secretObj := corev1.Secret{}
		err := r.client.Get(context.TODO(),
			types.NamespacedName{
				Namespace: cr.Namespace,
				Name:      cr.Spec.SecretsName,
			},
			&secretObj,
		)
		if err != nil {
			return errors.Wrap(err, "get cluster secret")
		}
		rootPass := string(secretObj.Data["root"])
		hosts := []string{cr.Name + "-pxc"}

		um, err := users.New(hosts, rootPass)
		if err != nil {
			usersSecretObj.Annotations["status"] = statusFailed
			errU := r.client.Update(context.TODO(), &usersSecretObj)
			if errU != nil {
				return errors.Wrap(errU, "update secret status")
			}
			return errors.Wrap(err, "new users manager")
		}

		err = um.GetUsersData(usersSecretObj, internalSecret)
		if err != nil {
			usersSecretObj.Annotations["status"] = statusFailed
			errU := r.client.Update(context.TODO(), &usersSecretObj)
			if errU != nil {
				return errors.Wrap(errU, "update secret status")
			}
			return errors.Wrap(err, "get users data")
		}

		err = um.ManageUsers()
		if err != nil {
			usersSecretObj.Annotations["status"] = statusFailed
			errU := r.client.Update(context.TODO(), &usersSecretObj)
			if errU != nil {
				return errors.Wrap(errU, "update secret status")
			}
			return errors.Wrap(err, "manage users")
		}

		if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Size > 0 {
			pod := corev1.Pod{}
			err = r.client.Get(context.TODO(),
				types.NamespacedName{
					Namespace: cr.Namespace,
					Name:      cr.Name + "-proxysql-0",
				},
				&pod,
			)
			if err != nil {
				return errors.Wrap(err, "get proxy pod")
			}
			var errb, outb bytes.Buffer
			err = r.clientcmd.Exec(&pod, "proxysql", []string{"proxysql-admin", "--syncusers"}, nil, &outb, &errb, false)
			if err != nil {
				return errors.Errorf("exec syncusers: %v / %s / %s", err, outb.String(), errb.String())
			}
			if len(errb.Bytes()) > 0 {
				return errors.New("syncusers: " + errb.String())
			}
			log.Info(outb.String())
		}

		for key, pass := range usersSecretObj.Data {
			if key == "grants.yaml" {
				continue
			}
			usersSecretObj.Annotations[key] = sha256Hash(pass)
		}
		usersSecretObj.Annotations["last-applied"] = newHash
		usersSecretObj.Annotations["status"] = statusSucceeded

		err = r.updateInternalSecret(cr, &usersSecretObj)
		if err != nil {
			return errors.Wrap(err, "update internal secret")
		}

		err = r.client.Update(context.TODO(), &usersSecretObj)
		if err != nil {
			return errors.Wrap(err, "update secret status")
		}
		return nil
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) getInternalSecret(cr *api.PerconaXtraDBCluster, userSecret *corev1.Secret) (corev1.Secret, error) {
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
		return secretObj, errors.Wrap(err, "get internal users secret")
	} else if k8serrors.IsNotFound(err) {
		secretObj, err = r.handleUsers(cr, userSecret, false)
		if err != nil {
			return secretObj, errors.Wrap(err, "handle internal secret")
		}

		err = r.client.Create(context.TODO(), &secretObj)
		if err != nil {
			return secretObj, errors.Wrap(err, "create internal users secret")
		}
		return secretObj, nil
	}

	return secretObj, nil
}

func (r *ReconcilePerconaXtraDBCluster) updateInternalSecret(cr *api.PerconaXtraDBCluster, userSecret *corev1.Secret) error {
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

func usersDataChanged(newHash *string, usersSecret *corev1.Secret) bool {
	if secretData, ok := usersSecret.Data["grants.yaml"]; ok {
		hash := sha256Hash(secretData)
		*newHash = hash
		if lastAppliedHash, ok := usersSecret.Annotations["last-applied"]; ok {
			if lastAppliedHash != hash {
				return true
			}
		}
	}

	return usersPasswordsChanged(usersSecret)
}

func usersPasswordsChanged(usersSecret *corev1.Secret) bool {
	for k, newPass := range usersSecret.Data {
		if k == "grants.yaml" {
			continue
		}
		oldPassHash := ""
		if hash, ok := usersSecret.Annotations[k]; ok {
			oldPassHash = hash
		}
		if sha256Hash(newPass) != oldPassHash {
			return true
		}
	}

	return false
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
		err = json.Unmarshal(secretObj.Data["users"], &interUsers)
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
				interUser.Status = "applied"
			} else {
				interUser.Status = "applying"
			}
			interUser.Time = time.Now().Unix()

			interUsers = append(interUsers, interUser)
		}
	}

	interUsersData, err := json.Marshal(interUsers)
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
