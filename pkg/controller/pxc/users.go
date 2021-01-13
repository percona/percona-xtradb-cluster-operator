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
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const internalPrefix = "internal-"

func (r *ReconcilePerconaXtraDBCluster) reconcileUsers(cr *api.PerconaXtraDBCluster) (pxcAnnotations, proxysqlAnnotations map[string]string, err error) {
	sysUsersSecretObj := corev1.Secret{}
	err = r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.SecretsName,
		},
		&sysUsersSecretObj,
	)
	if err != nil && k8serrors.IsNotFound(err) {
		return nil, nil, nil
	} else if err != nil {
		return nil, nil, errors.Wrapf(err, "get sys users secret '%s'", cr.Spec.SecretsName)
	}

	secretName := internalPrefix + cr.Name

	internalSysSecretObj := corev1.Secret{}

	err = r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      secretName,
		},
		&internalSysSecretObj,
	)
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, nil, errors.Wrap(err, "get internal sys users secret")
	}

	if k8serrors.IsNotFound(err) {
		internalSysUsersSecret := sysUsersSecretObj.DeepCopy()
		internalSysUsersSecret.ObjectMeta = metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cr.Namespace,
		}
		err = r.client.Create(context.TODO(), internalSysUsersSecret)
		if err != nil {
			return nil, nil, errors.Wrap(err, "create internal sys users secret")
		}
		return nil, nil, nil
	}

	if cr.Status.PXC.Ready > 0 {
		err := r.manageOperatorAdminUser(cr, &sysUsersSecretObj, &internalSysSecretObj)
		if err != nil {
			return nil, nil, errors.Wrap(err, "manage operator admin user")
		}
		if cr.CompareVersionWith("1.6.0") >= 0 {
			// monitor user need more grants for work in version more then 1.6.0
			err = r.manageMonitorUser(cr, &internalSysSecretObj)
			if err != nil {
				return nil, nil, errors.Wrap(err, "manage monitor user")
			}
		}
		if cr.CompareVersionWith("1.7.0") >= 0 {
			// xtrabackup user need more grants for work in version more then 1.7.0
			err = r.manageXtrabackupUser(cr, &internalSysSecretObj)
			if err != nil {
				return nil, nil, errors.Wrap(err, "manage xtrabackup user")
			}
		}
	}

	if cr.Status.Status != api.AppStateReady {
		return nil, nil, nil
	}

	newSysData, err := json.Marshal(sysUsersSecretObj.Data)
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal sys secret data")
	}
	newSecretDataHash := sha256Hash(newSysData)

	dataChanged, err := sysUsersSecretDataChanged(newSecretDataHash, &internalSysSecretObj)
	if err != nil {
		return nil, nil, errors.Wrap(err, "check sys users data changes")
	}

	if !dataChanged {
		return nil, nil, nil
	}

	restartPXC, restartProxy, err := r.manageSysUsers(cr, &sysUsersSecretObj, &internalSysSecretObj)
	if err != nil {
		return nil, nil, errors.Wrap(err, "manage sys users")
	}

	internalSysSecretObj.Data = sysUsersSecretObj.Data
	err = r.client.Update(context.TODO(), &internalSysSecretObj)
	if err != nil {
		return nil, nil, errors.Wrap(err, "update internal sys users secret")
	}

	if restartProxy {
		proxysqlAnnotations = make(map[string]string)
		proxysqlAnnotations["last-applied-secret"] = newSecretDataHash
	}
	if restartPXC {
		pxcAnnotations = make(map[string]string)
		pxcAnnotations["last-applied-secret"] = newSecretDataHash
	}

	return pxcAnnotations, proxysqlAnnotations, nil
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

	addr := cr.Name + "-pxc-unready." + cr.Namespace + ":3306"
	hasKey, err := cr.ConfigHasKey("mysqld", "proxy_protocol_networks")
	if err != nil {
		return errors.Wrap(err, "check if congfig has proxy_protocol_networks key")
	}
	if hasKey {
		addr = cr.Name + "-pxc-unready." + cr.Namespace + ":33062"
	}

	um, err := users.NewManager(addr, pxcUser, pxcPass)
	if err != nil {
		return errors.Wrap(err, "new users manager for grant")
	}
	defer um.Close()

	err = um.Update160MonitorUserGrant(string(internalSysSecretObj.Data["monitor"]))
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

func (r *ReconcilePerconaXtraDBCluster) manageXtrabackupUser(cr *api.PerconaXtraDBCluster, internalSysSecretObj *corev1.Secret) error {
	annotationName := "grant-for-1.7.0-xtrabackup-user"
	if internalSysSecretObj.Annotations[annotationName] == "done" {
		return nil
	}

	pxcUser := "root"
	pxcPass := string(internalSysSecretObj.Data["root"])
	if _, ok := internalSysSecretObj.Data["operator"]; ok {
		pxcUser = "operator"
		pxcPass = string(internalSysSecretObj.Data["operator"])
	}

	addr := cr.Name + "-pxc-unready." + cr.Namespace + ":3306"
	hasKey, err := cr.ConfigHasKey("mysqld", "proxy_protocol_networks")
	if err != nil {
		return errors.Wrap(err, "check if congfig has proxy_protocol_networks key")
	}
	if hasKey {
		addr = cr.Name + "-pxc-unready." + cr.Namespace + ":33062"
	}

	um, err := users.NewManager(addr, pxcUser, pxcPass)
	if err != nil {
		return errors.Wrap(err, "new users manager for grant")
	}
	defer um.Close()

	err = um.Update170XtrabackupUser(string(internalSysSecretObj.Data["xtrabackup"]))
	if err != nil {
		return errors.Wrap(err, "update xtrabackup grant")
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
	type action int

	const (
		rPXC action = 1 << iota
		rPXCifPMM
		rProxy
		rProxyifPMM
		syncProxyUsers
	)

	type user struct {
		name      string
		hosts     []string
		proxyUser bool
		action    action
	}
	requiredUsers := []user{
		{
			name:   "root",
			hosts:  []string{"localhost", "%"},
			action: syncProxyUsers,
		},
		{
			name:      "monitor",
			hosts:     []string{"%"},
			proxyUser: true,
			action:    rProxy | rPXCifPMM,
		},
		{
			name:  "clustercheck",
			hosts: []string{"localhost"},
		},
		{
			name:   "operator",
			hosts:  []string{"%"},
			action: rProxy,
		},
	}

	xtrabcupUser := user{
		name:   "xtrabackup",
		hosts:  []string{"localhost"},
		action: rPXC,
	}
	if cr.CompareVersionWith("1.7.0") >= 0 {
		xtrabcupUser.hosts = []string{"%"}
	}
	requiredUsers = append(requiredUsers, xtrabcupUser)

	if cr.Spec.PMM != nil && cr.Spec.PMM.Enabled {
		requiredUsers = append(requiredUsers, user{
			name:   "pmmserver",
			action: rProxyifPMM | rPXCifPMM,
		})
	}
	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		requiredUsers = append(requiredUsers, user{
			name:      "proxyadmin",
			proxyUser: true,
			action:    rProxy,
		})
	}

	var sysUsers, proxyUsers []users.SysUser
	var todo action
	for _, user := range requiredUsers {
		if len(sysUsersSecretObj.Data[user.name]) == 0 {
			return false, false, errors.New("undefined or not exist user " + user.name)
		}

		if bytes.Equal(sysUsersSecretObj.Data[user.name], internalSysSecretObj.Data[user.name]) {
			continue
		}

		todo |= user.action

		pass := string(sysUsersSecretObj.Data[user.name])

		if user.proxyUser {
			proxyUsers = append(proxyUsers, users.SysUser{Name: user.name, Pass: pass})
		}

		if len(user.hosts) != 0 {
			sysUsers = append(sysUsers, users.SysUser{
				Name:  user.name,
				Pass:  pass,
				Hosts: user.hosts,
			})
		}
	}

	// clear 'isPMM flags if PMM isn't enabled
	if cr.Spec.PMM == nil || !cr.Spec.PMM.Enabled {
		todo &^= rPXCifPMM | rProxyifPMM
	}

	restartPXC := todo&(rPXC|rPXCifPMM) != 0
	restartProxy := todo&(rProxy|rProxyifPMM) != 0

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
		return false, false, errors.Wrap(err, "new users manager")
	}
	defer um.Close()

	err = um.UpdateUsersPass(sysUsers)
	if err != nil {
		return false, false, errors.Wrap(err, "update sys users pass")
	}
	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		err = updateProxyUsers(proxyUsers, internalSysSecretObj, cr)
		if err != nil {
			return false, false, errors.Wrap(err, "update Proxy users pass")
		}
	}
	if todo&syncProxyUsers != 0 && !restartProxy {
		err = r.syncPXCUsersWithProxySQL(cr)
		if err != nil {
			return false, false, errors.Wrap(err, "sync users")
		}
	}

	return restartPXC, restartProxy, nil
}

func (r *ReconcilePerconaXtraDBCluster) syncPXCUsersWithProxySQL(cr *api.PerconaXtraDBCluster) error {
	if cr.Status.Status != api.AppStateReady || cr.Status.ProxySQL.Status != api.AppStateReady {
		return nil
	}
	// sync users if ProxySql enabled
	if cr.Spec.ProxySQL == nil || !cr.Spec.ProxySQL.Enabled || cr.Status.ObservedGeneration != cr.Generation || cr.Status.PXC.Ready < 1 {
		return nil
	}
	pod := corev1.Pod{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Name + "-proxysql-0",
		},
		&pod,
	)
	if err != nil && k8serrors.IsNotFound(err) {
		return err
	} else if err != nil {
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

	return nil
}

func updateProxyUsers(proxyUsers []users.SysUser, internalSysSecretObj *corev1.Secret, cr *api.PerconaXtraDBCluster) error {
	if len(proxyUsers) == 0 {
		return nil
	}

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

func (r *ReconcilePerconaXtraDBCluster) manageOperatorAdminUser(cr *api.PerconaXtraDBCluster, sysUsersSecretObj, internalSysSecretObj *corev1.Secret) error {
	pass, existInSys := sysUsersSecretObj.Data["operator"]
	_, existInInternal := internalSysSecretObj.Data["operator"]
	if existInSys && !existInInternal {
		if internalSysSecretObj.Data == nil {
			internalSysSecretObj.Data = make(map[string][]byte)
		}
		internalSysSecretObj.Data["operator"] = pass
		return nil
	}
	if existInSys {
		return nil
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
	internalSysSecretObj.Data["operator"] = pass

	err = r.client.Update(context.TODO(), sysUsersSecretObj)
	if err != nil {
		return errors.Wrap(err, "update sys users secret")
	}
	err = r.client.Update(context.TODO(), internalSysSecretObj)
	if err != nil {
		return errors.Wrap(err, "update internal users secret")
	}

	return nil
}

func sysUsersSecretDataChanged(newHash string, usersSecret *corev1.Secret) (bool, error) {
	secretData, err := json.Marshal(usersSecret.Data)
	if err != nil {
		return true, err
	}

	return sha256Hash(secretData) != newHash, nil
}

func sha256Hash(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
