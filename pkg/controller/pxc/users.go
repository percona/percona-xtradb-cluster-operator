package pxc

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
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

	err := r.handleSysUsersSecret(cr)
	if err != nil {
		log.Error(err, "handle system users secret")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleSysUsersSecret(cr *api.PerconaXtraDBCluster) error {
	sysUsersSecretObj := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.SecretsName,
		},
		&sysUsersSecretObj,
	)
	if err != nil && k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Wrapf(err, "get sys users secret '%s'", cr.Spec.SecretsName)
	}

	newSysData, err := json.Marshal(sysUsersSecretObj.Data)
	if err != nil {
		return errors.Wrap(err, "marshal sys secret data")
	}
	newSecretDataHash := sha256Hash(newSysData)

	internalSysSecretObj, err := r.getInternalSysUsersSecret(cr, &sysUsersSecretObj)
	if err != nil {
		return errors.Wrap(err, "get internal sys users secret")
	}

	dataChanged, err := sysUsersSecretDataChanged(newSecretDataHash, &internalSysSecretObj)
	if err != nil {
		return errors.Wrap(err, "check sys users data changes")
	}

	if !dataChanged {
		return nil
	}

	var restartPXC bool
	var restartProxy bool

	err = r.manageSysUsers(cr, &sysUsersSecretObj, &internalSysSecretObj, &restartPXC, &restartProxy)
	if err != nil {
		return errors.Wrap(err, "manage sys users")
	}

	err = r.updateInternalSysUsersSecret(cr, &sysUsersSecretObj)
	if err != nil {
		return errors.Wrap(err, "update internal sys users secret")
	}

	if restartProxy && cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Size > 0 {
		err = r.restartProxy(cr, newSecretDataHash)
		if err != nil {
			return errors.Wrap(err, "restart proxy")
		}
	}

	if restartPXC {
		err = r.restartPXC(cr, newSecretDataHash, &sysUsersSecretObj)
		if err != nil {
			return errors.Wrap(err, "restart pxc")
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) manageSysUsers(cr *api.PerconaXtraDBCluster, sysUsersSecretObj, internalSysSecretObj *corev1.Secret, restartPXC, restartProxy *bool) error {
	um, err := users.NewManager([]string{cr.Name + "-pxc"}, string(internalSysSecretObj.Data["root"]))
	if err != nil {
		return errors.Wrap(err, "new users manager")
	}
	var sysUsers []users.SysUser

	for name, pass := range sysUsersSecretObj.Data {
		hosts := []string{}

		if string(sysUsersSecretObj.Data[name]) == string(internalSysSecretObj.Data[name]) {
			continue
		}

		switch name {
		case "root":
			*restartProxy = true
			hosts = []string{"localhost", "%"}
		case "xtrabackup":
			*restartProxy = true
			*restartPXC = true
			hosts = []string{"localhost"}
		case "monitor":
			if cr.Spec.PMM.Enabled {
				*restartProxy = true
				*restartPXC = true
				hosts = []string{"10.%", "%"}
			}
		case "clustercheck":
			*restartProxy = true
			*restartPXC = true
			hosts = []string{"localhost"}
		case "proxyadmin":
			*restartProxy = true
			continue

		case "pmmserver":
			if cr.Spec.PMM.Enabled {
				*restartProxy = true
				*restartPXC = true
				continue
			}
		}
		user := users.SysUser{
			Name:  name,
			Pass:  string(pass),
			Hosts: hosts,
		}
		sysUsers = append(sysUsers, user)
	}

	if len(sysUsers) > 0 {
		err = um.UpdateUsersPass(sysUsers)
		if err != nil {
			return errors.Wrap(err, "update sys users pass")
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) restartPXC(cr *api.PerconaXtraDBCluster, newSecretDataHash string, sysUsersSecretObj *corev1.Secret) error {
	sfsPXC := appsv1.StatefulSet{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Name + "-pxc",
		},
		&sfsPXC,
	)

	if len(sfsPXC.Annotations) == 0 {
		sfsPXC.Annotations = make(map[string]string)
	}
	sfsPXC.Spec.Template.Annotations["last-applied-secret"] = newSecretDataHash

	err = r.client.Update(context.TODO(), &sfsPXC)
	if err != nil {
		return errors.Wrap(err, "update pxc sfs last-applied annotation")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) restartProxy(cr *api.PerconaXtraDBCluster, newSecretDataHash string) error {
	pvcProxy := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cr.Namespace,
			Name:      "proxydata-" + cr.Name + "-proxysql-0",
		},
	}
	err := r.client.Delete(context.TODO(), &pvcProxy)
	if err != nil {
		return errors.Wrap(err, "delete proxy pvc")
	}

	sfsProxy := appsv1.StatefulSet{}
	err = r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Name + "-proxysql",
		},
		&sfsProxy,
	)
	if err != nil {
		return errors.Wrap(err, "get proxy statefulset")
	}

	err = r.client.Delete(context.TODO(), &sfsProxy)
	if err != nil {
		return errors.Wrap(err, "delete proxy statefulset")
	}

	return nil
}

// getInternalSysUsersSecret return secret created by operator for storing system users data
func (r *ReconcilePerconaXtraDBCluster) getInternalSysUsersSecret(cr *api.PerconaXtraDBCluster, sysUsersSecretObj *corev1.Secret) (corev1.Secret, error) {
	secretName := internalPrefix + cr.Spec.SecretsName
	internalSysUsersSecretObj, err := r.getInternalSysUsersSecretObj(cr, sysUsersSecretObj)
	if err != nil {
		return internalSysUsersSecretObj, errors.Wrap(err, "create internal sys users secret object")
	}
	err = r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      secretName,
		},
		&internalSysUsersSecretObj,
	)
	if err != nil && !k8serrors.IsNotFound(err) {
		return internalSysUsersSecretObj, errors.Wrap(err, "get internal sys users secret")
	} else if k8serrors.IsNotFound(err) {
		err = r.client.Create(context.TODO(), &internalSysUsersSecretObj)
		if err != nil {
			return internalSysUsersSecretObj, errors.Wrap(err, "create internal sys users secret")
		}
	}

	return internalSysUsersSecretObj, nil
}

func (r *ReconcilePerconaXtraDBCluster) updateInternalSysUsersSecret(cr *api.PerconaXtraDBCluster, sysUsersSecretObj *corev1.Secret) error {
	internalAppUsersSecretObj, err := r.getInternalSysUsersSecretObj(cr, sysUsersSecretObj)
	if err != nil {
		return errors.Wrap(err, "get internal sys users secret object")
	}
	err = r.client.Update(context.TODO(), &internalAppUsersSecretObj)
	if err != nil {
		return errors.Wrap(err, "create internal sys users secret")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) getInternalSysUsersSecretObj(cr *api.PerconaXtraDBCluster, sysUsersSecretObj *corev1.Secret) (corev1.Secret, error) {
	internalSysUsersSecretObj := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      internalPrefix + cr.Spec.SecretsName,
			Namespace: cr.Namespace,
		},
		Data: sysUsersSecretObj.Data,
		Type: corev1.SecretTypeOpaque,
	}
	err := setControllerReference(cr, &internalSysUsersSecretObj, r.scheme)
	if err != nil {
		return internalSysUsersSecretObj, errors.Wrap(err, "set owner refs")
	}

	return internalSysUsersSecretObj, nil
}

func sysUsersSecretDataChanged(newHash string, usersSecret *corev1.Secret) (bool, error) {
	secretData, err := json.Marshal(usersSecret.Data)
	if err != nil {
		return true, err
	}
	oldHash := sha256Hash(secretData)

	if oldHash != newHash {
		return true, nil
	}

	return false, nil
}

func sha256Hash(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
