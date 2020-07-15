package pxc

import (
	"bytes"
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
	internalSysSecretObj, sysUsersSecretObj, err := r.getSysUsersSecrets(cr)
	if err != nil {
		return errors.Wrap(err, "get internal sys users secret")
	}

	if cr.Status.Status != api.AppStateReady {
		return nil
	}

	if cr.Status.PXC.Ready > 0 {
		err := r.manageOperatorAdminUser(cr)
		if err != nil {
			return errors.Wrap(err, "manage operator admin user")
		}
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

	if cr.CompareVersionWith("1.6.0") >= 0 {
		// monitor user need more grants for work in version more then 1.6.0
		err = r.manageMonitorUser(cr, &internalSysSecretObj)
		if err != nil {
			return errors.Wrap(err, "manage monitor user")
		}
	}

	dataChanged, err := sysUsersSecretDataChanged(newSecretDataHash, &internalSysSecretObj)
	if err != nil {
		return errors.Wrap(err, "check sys users data changes")
	}

	if !dataChanged {
		return nil
	}

	restartPXC, restartProxy, err := r.manageSysUsers(cr, &sysUsersSecretObj, &internalSysSecretObj)
	if err != nil {
		return errors.Wrap(err, "manage sys users")
	}

	err = r.updateInternalSysUsersSecret(cr, &sysUsersSecretObj)
	if err != nil {
		return errors.Wrap(err, "update internal sys users secret")
	}

	if restartProxy && cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		err = r.restartProxy(cr, newSecretDataHash)
		if err != nil {
			return errors.Wrap(err, "restart proxy")
		}
	}

	if restartPXC {
		err = r.restartPXC(cr, newSecretDataHash)
		if err != nil {
			return errors.Wrap(err, "restart pxc")
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) manageMonitorUser(cr *api.PerconaXtraDBCluster, internalSysSecretObj *corev1.Secret) error {
	annotationName := "grant-for-1.6.0-monitor-user"
	if internalSysSecretObj.Annotations[annotationName] == "done" {
		return nil
	}

	pxcUser := "root"
	pxcPass := string(internalSysSecretObj.Data["root"])
	if _, ok := internalSysSecretObj.Data["operator"]; ok {
		pxcUser = "operator"
		pxcPass = string(internalSysSecretObj.Data["operator"])
	}

	um, err := users.NewManager(cr.Name+"-pxc-unready."+cr.Namespace+":33062", pxcUser, pxcPass)
	if err != nil {
		return errors.Wrap(err, "new users manager for grant")
	}
	defer um.Close()

	err = um.Update160MonitorUserGrant()
	if err != nil {
		return errors.Wrap(err, "update monitor grant")
	}

	if internalSysSecretObj.Annotations == nil {
		internalSysSecretObj.Annotations = make(map[string]string)
	}

	internalSysSecretObj.Annotations[annotationName] = "done"
	err = r.client.Update(context.TODO(), internalSysSecretObj)
	if err != nil {
		return errors.Wrap(err, "update internal sys users secret annotation")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) manageSysUsers(cr *api.PerconaXtraDBCluster, sysUsersSecretObj, internalSysSecretObj *corev1.Secret) (bool, bool, error) {
	var restartPXC, restartProxy, syncProxySQLUsers bool

	var sysUsers []users.SysUser
	var proxyUsers []users.SysUser

	for name, pass := range sysUsersSecretObj.Data {
		hosts := []string{}

		if string(sysUsersSecretObj.Data[name]) == string(internalSysSecretObj.Data[name]) {
			continue
		}

		switch name {
		case "root":
			syncProxySQLUsers = true
			hosts = []string{"localhost", "%"}
		case "xtrabackup":
			restartPXC = true
			hosts = []string{"localhost"}
		case "monitor":
			restartProxy = true
			proxyUsers = append(proxyUsers, users.SysUser{Name: name, Pass: string(pass)})
			if cr.Spec.PMM.Enabled {
				restartPXC = true
			}
			hosts = []string{"%"}
		case "clustercheck":
			restartPXC = true
			hosts = []string{"localhost"}
		case "proxyadmin":
			restartProxy = true
			proxyUsers = append(proxyUsers, users.SysUser{Name: name, Pass: string(pass)})
			continue
		case "pmmserver":
			if cr.Spec.PMM.Enabled {
				restartProxy = true
				restartPXC = true
				continue
			}
		case "operator":
			restartProxy = true
			hosts = []string{"%"}
		}
		user := users.SysUser{
			Name:  name,
			Pass:  string(pass),
			Hosts: hosts,
		}
		sysUsers = append(sysUsers, user)
	}

	pxcUser := "root"
	pxcPass := string(internalSysSecretObj.Data["root"])
	if _, ok := sysUsersSecretObj.Data["operator"]; ok {
		pxcUser = "operator"
		pxcPass = string(internalSysSecretObj.Data["operator"])
	}

	addr := cr.Name + "-pxc." + cr.Namespace
	if cr.CompareVersionWith("1.6.0") >= 0 {
		addr = cr.Name + "-pxc-unready." + cr.Namespace + ":33062"
	}
	um, err := users.NewManager(addr, pxcUser, pxcPass)
	if err != nil {
		return restartPXC, restartProxy, errors.Wrap(err, "new users manager")
	}
	defer um.Close()

	if len(sysUsers) > 0 {
		err = um.UpdateUsersPass(sysUsers)
		if err != nil {
			return restartPXC, restartProxy, errors.Wrap(err, "update sys users pass")
		}
	}

	if len(proxyUsers) > 0 {
		err = updateProxyUsers(proxyUsers, internalSysSecretObj, cr)
		if err != nil {
			return restartPXC, restartProxy, errors.Wrap(err, "update Proxy users pass")
		}
	}

	if syncProxySQLUsers && !restartProxy {
		err = r.syncPXCUsersWithProxySQL(cr)
		if err != nil {
			return restartPXC, restartProxy, errors.Wrap(err, "sync users")
		}
	}

	return restartPXC, restartProxy, nil
}

func (r *ReconcilePerconaXtraDBCluster) syncPXCUsersWithProxySQL(cr *api.PerconaXtraDBCluster) error {
	if cr.Status.Status != api.AppStateReady || cr.Status.ProxySQL.Status == api.AppStateReady {
		return nil
	}
	// sync users if ProxySql enabled
	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		pod := corev1.Pod{}
		err := r.client.Get(context.TODO(),
			types.NamespacedName{
				Namespace: cr.Namespace,
				Name:      cr.Name + "-proxysql-0",
			},
			&pod,
		)
		if err != nil {
			return errors.Wrap(err, "get proxysql pod")
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

func (r *ReconcilePerconaXtraDBCluster) restartPXC(cr *api.PerconaXtraDBCluster, newSecretDataHash string) error {
	sfsPXC := appsv1.StatefulSet{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Name + "-pxc",
		},
		&sfsPXC,
	)
	if err != nil {
		return errors.Wrap(err, "failed to get stetefulset")
	}

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

func updateProxyUsers(proxyUsers []users.SysUser, internalSysSecretObj *corev1.Secret, cr *api.PerconaXtraDBCluster) error {
	um, err := users.NewManager(cr.Name+"-proxysql-unready."+cr.Namespace+":6032", "proxyadmin", string(internalSysSecretObj.Data["proxyadmin"]))
	if err != nil {
		return errors.Wrap(err, "new users manager")
	}
	defer um.Close()

	err = um.UpdateProxyUsers(proxyUsers)
	if err != nil {
		return errors.Wrap(err, "update proxy users")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) restartProxy(cr *api.PerconaXtraDBCluster, newSecretDataHash string) error {
	sfsProxy := appsv1.StatefulSet{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Name + "-proxysql",
		},
		&sfsProxy,
	)
	if err != nil {
		return errors.Wrap(err, "failed to get proxysql statefulset")
	}

	if len(sfsProxy.Annotations) == 0 {
		sfsProxy.Annotations = make(map[string]string)
	}
	sfsProxy.Spec.Template.Annotations["last-applied-secret"] = newSecretDataHash

	err = r.client.Update(context.TODO(), &sfsProxy)
	if err != nil {
		return errors.Wrap(err, "update proxy sfs last-applied annotation")
	}

	return nil
}

// getSysUsersSecrets return internal and external secrets for storing system users data
func (r *ReconcilePerconaXtraDBCluster) getSysUsersSecrets(cr *api.PerconaXtraDBCluster) (internalSysUsersSecretObj, sysUsersSecretObj corev1.Secret, err error) {
	sysUsersSecretObj = corev1.Secret{}
	err = r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.SecretsName,
		},
		&sysUsersSecretObj,
	)
	if err != nil && k8serrors.IsNotFound(err) {
		return corev1.Secret{}, corev1.Secret{}, nil
	} else if err != nil {
		return corev1.Secret{}, corev1.Secret{}, errors.Wrapf(err, "get sys users secret '%s'", cr.Spec.SecretsName)
	}
	secretName := internalPrefix + cr.Name
	internalSysUsersSecretObj, err = r.getInternalSysUsersSecretObj(cr, &sysUsersSecretObj)
	if err != nil {
		return internalSysUsersSecretObj, sysUsersSecretObj, errors.Wrap(err, "create internal sys users secret object")
	}
	err = r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      secretName,
		},
		&internalSysUsersSecretObj,
	)
	if err != nil && !k8serrors.IsNotFound(err) {
		return internalSysUsersSecretObj, sysUsersSecretObj, errors.Wrap(err, "get internal sys users secret")
	}

	if k8serrors.IsNotFound(err) {
		err = r.client.Create(context.TODO(), &internalSysUsersSecretObj)
		if err != nil {
			return internalSysUsersSecretObj, sysUsersSecretObj, errors.Wrap(err, "create internal sys users secret")
		}
	}

	return internalSysUsersSecretObj, sysUsersSecretObj, nil
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
			Name:      internalPrefix + cr.Name,
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

func (r *ReconcilePerconaXtraDBCluster) manageOperatorAdminUser(cr *api.PerconaXtraDBCluster) error {
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

	for name := range sysUsersSecretObj.Data {
		if name == "operator" {
			return nil
		}
	}

	pass, err := generatePass()
	if err != nil {
		return errors.Wrap(err, "generate password")
	}
	addr := cr.Name + "-pxc." + cr.Namespace
	if cr.CompareVersionWith("1.6.0") >= 0 {
		addr = cr.Name + "-pxc-unready." + cr.Namespace + ":33062"
	}
	um, err := users.NewManager(addr, "root", string(sysUsersSecretObj.Data["root"]))
	if err != nil {
		return errors.Wrap(err, "new users manager")
	}
	defer um.Close()

	err = um.CreateOperatorUser(string(pass))
	if err != nil {
		return errors.Wrap(err, "create operator user")
	}

	sysUsersSecretObj.Data["operator"] = pass
	err = r.client.Update(context.TODO(), &sysUsersSecretObj)
	if err != nil {
		return errors.Wrap(err, "update sys users secret")
	}

	return nil
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
