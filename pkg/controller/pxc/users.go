package pxc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
		err := r.handleAppUsersSecret(secretName, cr)
		if err != nil {
			log.Error(err, "handle users secret "+secretName)
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleAppUsersSecret(secretName string, cr *api.PerconaXtraDBCluster) error {
	appUsersSecretObj := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      secretName,
		},
		&appUsersSecretObj,
	)
	if err != nil && k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Wrapf(err, "get app users secret '%s'", secretName)
	}

	newSecretDataHash := ""

	dataChanged, err := appUsersSecretDataChanged(&newSecretDataHash, &appUsersSecretObj)
	if err != nil {
		return errors.Wrap(err, "check app users data changes")
	}

	if !dataChanged {
		if appUsersSecretObj.Annotations["status"] == statusSucceeded || appUsersSecretObj.Annotations["status"] == statusFailed {
			return nil
		}
	}

	if len(appUsersSecretObj.Annotations) == 0 {
		appUsersSecretObj.Annotations = make(map[string]string)
	}

	if dataChanged {
		err = r.manageAppUsers(cr, &appUsersSecretObj)
		if err != nil {
			return errors.Wrap(err, "manage client users")
		}

		appUsersSecretObj.Annotations["last-applied"] = newSecretDataHash
		appUsersSecretObj.Annotations["status"] = statusSucceeded

		err = r.updateInternalAppUsersSecret(cr, &appUsersSecretObj)
		if err != nil {
			return errors.Wrap(err, "update internal app users secret")
		}

		err = r.client.Update(context.TODO(), &appUsersSecretObj)
		if err != nil {
			return errors.Wrap(err, "update app users secret status")
		}
		return nil
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) manageAppUsers(cr *api.PerconaXtraDBCluster, appUsersSecretObj *corev1.Secret) error {
	sysUsersSecretObj := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.SecretsName,
		},
		&sysUsersSecretObj,
	)
	if err != nil {
		return errors.Wrap(err, "get sys users secret")
	}

	um, err := users.NewManager([]string{cr.Name + "-pxc"}, string(sysUsersSecretObj.Data["root"]))
	if err != nil {
		appUsersSecretObj.Annotations["status"] = statusFailed
		errU := r.client.Update(context.TODO(), appUsersSecretObj)
		if errU != nil {
			return errors.Wrap(errU, "update secret status")
		}
		return errors.Wrap(err, "new users manager")
	}

	internalAppSecretObj, err := r.getInternalAppUsersSecret(cr, appUsersSecretObj)
	if err != nil {
		return errors.Wrap(err, "internal secret")
	}

	err = um.GetUsersData(*appUsersSecretObj, internalAppSecretObj)
	if err != nil {
		appUsersSecretObj.Annotations["status"] = statusFailed
		errU := r.client.Update(context.TODO(), appUsersSecretObj)
		if errU != nil {
			return errors.Wrap(errU, "update secret status")
		}
		return errors.Wrap(err, "get users data")
	}

	err = um.ManageUsers(appUsersSecretObj.Name)
	if err != nil {
		appUsersSecretObj.Annotations["status"] = statusFailed
		errU := r.client.Update(context.TODO(), appUsersSecretObj)
		if errU != nil {
			return errors.Wrap(errU, "update secret status")
		}
		return errors.Wrap(err, "manage users")
	}

	// sync users if ProxySql enabled
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
	}

	return nil
}

// getInternalAppUsersSecret return secret created by operator for storing app users data and statuses
func (r *ReconcilePerconaXtraDBCluster) getInternalAppUsersSecret(cr *api.PerconaXtraDBCluster, appUsersSecretObj *corev1.Secret) (corev1.Secret, error) {
	secretName := internalPrefix + cr.Spec.SecretsName
	internalAppUsersSecretObj := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      secretName,
		},
		&internalAppUsersSecretObj,
	)
	if err != nil && !k8serrors.IsNotFound(err) {
		return internalAppUsersSecretObj, errors.Wrap(err, "get internal users secret")
	} else if k8serrors.IsNotFound(err) {
		internalAppUsersSecretObj, err = r.handleInternalAppUsersSecretObj(cr, appUsersSecretObj, false)
		if err != nil {
			return internalAppUsersSecretObj, errors.Wrap(err, "handle internal secret")
		}

		err = r.client.Create(context.TODO(), &internalAppUsersSecretObj)
		if err != nil {
			return internalAppUsersSecretObj, errors.Wrap(err, "create internal users secret")
		}
	}

	return internalAppUsersSecretObj, nil
}

func (r *ReconcilePerconaXtraDBCluster) updateInternalAppUsersSecret(cr *api.PerconaXtraDBCluster, appUsersSecretObj *corev1.Secret) error {
	internalAppUsersSecretObj, err := r.handleInternalAppUsersSecretObj(cr, appUsersSecretObj, true)
	if err != nil {
		return errors.Wrap(err, "update internal secret")
	}
	err = r.client.Update(context.TODO(), &internalAppUsersSecretObj)
	if err != nil {
		return errors.Wrap(err, "create internal users secret")
	}
	return nil

}

func appUsersSecretDataChanged(newHash *string, usersSecret *corev1.Secret) (bool, error) {
	secretData, err := json.Marshal(usersSecret.Data)
	if err != nil {
		return true, err
	}
	hash := sha256Hash(secretData)
	*newHash = hash
	if lastAppliedHash, ok := usersSecret.Annotations["last-applied"]; ok {
		if lastAppliedHash != hash {
			return true, nil
		}
	}

	return false, nil
}

func (r *ReconcilePerconaXtraDBCluster) handleInternalAppUsersSecretObj(cr *api.PerconaXtraDBCluster, appUsersSecretObj *corev1.Secret, updateIntUsersList bool) (corev1.Secret, error) {
	interUsers := []users.InternalUser{}
	interUsersSecretObj := corev1.Secret{}
	intSecretName := internalPrefix + cr.Spec.SecretsName

	if updateIntUsersList { //here we are getting list of users that already stored in internal secret, and not belong to current owner
		err := r.client.Get(context.TODO(),
			types.NamespacedName{
				Namespace: cr.Namespace,
				Name:      intSecretName,
			},
			&interUsersSecretObj,
		)
		if err != nil {
			return interUsersSecretObj, errors.Wrap(err, "get internal users secret")
		}
		err = json.Unmarshal(interUsersSecretObj.Data["users"], &interUsers)
		if err != nil {
			return interUsersSecretObj, errors.Wrap(err, "unmarshal users secret data")
		}
		var newInterUsers []users.InternalUser
		for _, u := range interUsers {
			if u.Owner == appUsersSecretObj.Name {
				continue // we drop this user because we are going to update only current owner users list
			}
			newInterUsers = append(newInterUsers, u)
		}
		interUsers = newInterUsers
	}

	var usersSlice users.Data
	usersData := appUsersSecretObj.Data["grants.yaml"]
	err := yaml.Unmarshal(usersData, &usersSlice)
	if err != nil {
		return interUsersSecretObj, errors.Wrap(err, "unmarshal users secret data")
	}

	for _, user := range usersSlice.Users {
		for _, host := range user.Hosts {
			var interUser users.InternalUser
			interUser.Name = user.Name + "@" + host
			interUser.Owner = appUsersSecretObj.Name
			if updateIntUsersList {
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
		return interUsersSecretObj, errors.Wrap(err, "marshal internal users")
	}

	data := make(map[string][]byte)
	data["users"] = interUsersData

	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intSecretName,
			Namespace: cr.Namespace,
		},
		Data: data,
		Type: corev1.SecretTypeOpaque,
	}, nil
}

func sha256Hash(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
