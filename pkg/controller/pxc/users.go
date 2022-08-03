package pxc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

const internalPrefix = "internal-"

// https://dev.mysql.com/doc/refman/8.0/en/privileges-provided.html#priv_system-user
var privSystemUserAddedIn = version.Must(version.NewVersion("8.0.16"))

type userUpdateRestart struct {
	restartPXC            bool
	restartProxy          bool
	updateReplicationPass bool
}

type ReconcileUsersResult struct {
	pxcAnnotations            map[string]string
	proxysqlAnnotations       map[string]string
	updateReplicationPassword bool
}

func (r *ReconcilePerconaXtraDBCluster) reconcileUsers(cr *api.PerconaXtraDBCluster) (*ReconcileUsersResult, error) {
	sysUsersSecretObj := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.SecretsName,
		},
		&sysUsersSecretObj,
	)
	if err != nil && k8serrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Wrapf(err, "get sys users secret '%s'", cr.Spec.SecretsName)
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
		return nil, errors.Wrap(err, "get internal sys users secret")
	}

	if k8serrors.IsNotFound(err) {
		internalSysUsersSecret := sysUsersSecretObj.DeepCopy()
		internalSysUsersSecret.ObjectMeta = metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cr.Namespace,
		}
		err = r.client.Create(context.TODO(), internalSysUsersSecret)
		if err != nil {
			return nil, errors.Wrap(err, "create internal sys users secret")
		}
		return nil, nil
	}

	if cr.Status.PXC.Ready > 0 {
		err := r.manageOperatorAdminUser(cr, &sysUsersSecretObj, &internalSysSecretObj)
		if err != nil {
			return nil, errors.Wrap(err, "manage operator admin user")
		}
		if cr.CompareVersionWith("1.6.0") >= 0 {
			// monitor user need more grants for work in version more then 1.6.0
			err = r.manageMonitorUser(cr, &internalSysSecretObj)
			if err != nil {
				return nil, errors.Wrap(err, "manage monitor user")
			}
		}
		if cr.CompareVersionWith("1.7.0") >= 0 {
			// xtrabackup user need more grants for work in version more then 1.7.0
			err = r.manageXtrabackupUser(cr, &internalSysSecretObj)
			if err != nil {
				return nil, errors.Wrap(err, "manage xtrabackup user")
			}
		}
		if cr.CompareVersionWith("1.9.0") >= 0 {
			err = r.manageReplicationUser(cr, &sysUsersSecretObj, &internalSysSecretObj)
			if err != nil {
				return nil, errors.Wrap(err, "manage replication user")
			}
		}

		if cr.CompareVersionWith("1.10.0") >= 0 {
			mysqlVersion := cr.Status.PXC.Version
			if mysqlVersion == "" {
				mysqlVersion, err = r.mysqlVersion(cr, statefulset.NewNode(cr))
				if err != nil && !errors.Is(err, versionNotReadyErr) {
					return nil, errors.Wrap(err, "retrieving pxc version")
				}
			}

			if mysqlVersion != "" {
				ver, err := version.NewVersion(mysqlVersion)
				if err != nil {
					return nil, errors.Wrap(err, "invalid pxc version")
				}

				if !ver.LessThan(privSystemUserAddedIn) {
					if err := r.grantSystemUserPrivilege(cr, &internalSysSecretObj); err != nil {
						return nil, errors.Wrap(err, "grant system privilege")
					}
				}
			}
		}
	}

	if cr.Status.Status != api.AppStateReady {
		return nil, nil
	}

	newSysData, err := json.Marshal(sysUsersSecretObj.Data)
	if err != nil {
		return nil, errors.Wrap(err, "marshal sys secret data")
	}
	newSecretDataHash := sha256Hash(newSysData)

	dataChanged, err := sysUsersSecretDataChanged(newSecretDataHash, &internalSysSecretObj)
	if err != nil {
		return nil, errors.Wrap(err, "check sys users data changes")
	}

	if !dataChanged {
		return nil, nil
	}

	if _, ok := sysUsersSecretObj.Data["pmmserverkey"]; ok {
		if _, ok := internalSysSecretObj.Data["pmmserverkey"]; !ok {
			internalSysSecretObj.Data["pmmserverkey"] = sysUsersSecretObj.Data["pmmserverkey"]
		}
	}

	restarts, err := r.manageSysUsers(cr, &sysUsersSecretObj, &internalSysSecretObj)
	if err != nil {
		return nil, errors.Wrap(err, "manage sys users")
	}

	internalSysSecretObj.Data = sysUsersSecretObj.Data
	err = r.client.Update(context.TODO(), &internalSysSecretObj)
	if err != nil {
		return nil, errors.Wrap(err, "update internal sys users secret")
	}

	result := &ReconcileUsersResult{
		updateReplicationPassword: restarts.updateReplicationPass,
	}

	if restarts.restartProxy {
		result.proxysqlAnnotations = map[string]string{"last-applied-secret": newSecretDataHash}
	}
	if restarts.restartPXC {
		result.pxcAnnotations = map[string]string{"last-applied-secret": newSecretDataHash}
	}

	return result, nil
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

	um, err := users.NewManager(addr, pxcUser, pxcPass, cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
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

	um, err := users.NewManager(addr, pxcUser, pxcPass, cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
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

func (r *ReconcilePerconaXtraDBCluster) grantSystemUserPrivilege(cr *api.PerconaXtraDBCluster, internalSysSecretObj *corev1.Secret) error {
	annotationName := "grant-for-1.10.0-system-privilege"
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

	um, err := users.NewManager(addr, pxcUser, pxcPass, cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
	if err != nil {
		return errors.Wrap(err, "new users manager for grant")
	}
	defer um.Close()

	if err = um.Update1100SystemUserPrivilege(); err != nil {
		return errors.Wrap(err, "grant system user privilege")
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

func (r *ReconcilePerconaXtraDBCluster) manageSysUsers(cr *api.PerconaXtraDBCluster, sysUsersSecretObj, internalSysSecretObj *corev1.Secret) (*userUpdateRestart, error) {
	type action int

	const (
		rPXC action = 1 << iota
		rPXCifPMM
		rProxy
		rProxyifPMM
		syncProxyUsers
		syncReplicaUser
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

	xtrabackupUser := user{
		name:   "xtrabackup",
		hosts:  []string{"localhost"},
		action: rPXC,
	}
	if cr.CompareVersionWith("1.7.0") >= 0 {
		xtrabackupUser.hosts = []string{"%"}
	}
	requiredUsers = append(requiredUsers, xtrabackupUser)

	if cr.CompareVersionWith("1.9.0") >= 0 {
		requiredUsers = append(requiredUsers, user{
			name:   "replication",
			hosts:  []string{"%"},
			action: syncReplicaUser,
		})
	}

	if cr.Spec.PMM != nil && cr.Spec.PMM.Enabled {
		name := "pmmserverkey"
		if !cr.Spec.PMM.UseAPI(sysUsersSecretObj) {
			name = "pmmserver"
		}
		requiredUsers = append(requiredUsers, user{
			name:   name,
			action: rProxyifPMM | rPXCifPMM,
		})
	}
	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		requiredUsers = append(requiredUsers, user{
			name:      "proxyadmin",
			proxyUser: true,
			action:    rProxy | syncProxyUsers,
		})
	}

	var (
		sysUsers, proxyUsers []users.SysUser
		todo                 action
	)

	for _, user := range requiredUsers {
		if len(sysUsersSecretObj.Data[user.name]) == 0 {
			return nil, errors.New("undefined or not exist user " + user.name)
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

	res := &userUpdateRestart{
		restartPXC:            todo&(rPXC|rPXCifPMM) != 0,
		restartProxy:          todo&(rProxy|rProxyifPMM) != 0,
		updateReplicationPass: todo&syncReplicaUser != 0,
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
	um, err := users.NewManager(addr, pxcUser, pxcPass, cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
	if err != nil {
		return res, errors.Wrap(err, "new users manager")
	}
	defer um.Close()

	err = um.UpdateUsersPass(sysUsers)
	if err != nil {
		return res, errors.Wrap(err, "update sys users pass")
	}
	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		err = r.updateProxyUsers(proxyUsers, internalSysSecretObj, cr)
		if err != nil {
			return res, errors.Wrap(err, "update Proxy users pass")
		}
	}
	if todo&syncProxyUsers != 0 && !res.restartProxy {
		err = r.syncPXCUsersWithProxySQL(cr)
		if err != nil {
			return res, errors.Wrap(err, "sync users")
		}
	}

	return res, nil
}

func (r *ReconcilePerconaXtraDBCluster) syncPXCUsersWithProxySQL(cr *api.PerconaXtraDBCluster) error {
	if cr.Spec.ProxySQL == nil || !cr.Spec.ProxySQL.Enabled || cr.Status.PXC.Ready < 1 {
		return nil
	}
	if cr.Status.Status != api.AppStateReady || cr.Status.ProxySQL.Status != api.AppStateReady {
		return nil
	}

	for i := 0; i < int(cr.Spec.ProxySQL.Size); i++ {
		pod := corev1.Pod{}
		err := r.client.Get(context.TODO(),
			types.NamespacedName{
				Namespace: cr.Namespace,
				Name:      cr.Name + "-proxysql-" + strconv.Itoa(i),
			},
			&pod,
		)
		if err != nil && k8serrors.IsNotFound(err) {
			return err
		} else if err != nil {
			return errors.Wrap(err, "get proxysql pod")
		}
		var errb, outb bytes.Buffer
		err = r.clientcmd.Exec(&pod, "proxysql", []string{"percona-scheduler-admin", "--config-file=/etc/config.toml", "--syncusers", "--add-query-rule"}, nil, &outb, &errb, false)
		if err != nil {
			return errors.Errorf("exec syncusers: %v / %s / %s", err, outb.String(), errb.String())
		}
		if len(errb.Bytes()) > 0 {
			return errors.New("syncusers: " + errb.String())
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) updateProxyUsers(proxyUsers []users.SysUser, internalSysSecretObj *corev1.Secret, cr *api.PerconaXtraDBCluster) error {
	if len(proxyUsers) == 0 {
		return nil
	}
	for i := 0; i < int(cr.Spec.ProxySQL.Size); i++ {
		um, err := users.NewManager(cr.Name+"-proxysql-"+strconv.Itoa(i)+"."+cr.Name+"-proxysql-unready."+cr.Namespace+":6032", "proxyadmin", string(internalSysSecretObj.Data["proxyadmin"]), cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
		if err != nil {
			return errors.Wrap(err, "new users manager")
		}
		defer um.Close()
		err = um.UpdateProxyUsers(proxyUsers)
		if err != nil {
			return errors.Wrap(err, "update proxy users")
		}
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
	um, err := users.NewManager(addr, "root", string(sysUsersSecretObj.Data["root"]), cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
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

func (r *ReconcilePerconaXtraDBCluster) manageReplicationUser(cr *api.PerconaXtraDBCluster, sysUsersSecretObj, internalSysSecretObj *corev1.Secret) error {
	pass, existInSys := sysUsersSecretObj.Data["replication"]
	_, existInInternal := internalSysSecretObj.Data["replication"]
	if existInSys && !existInInternal {
		if internalSysSecretObj.Data == nil {
			internalSysSecretObj.Data = make(map[string][]byte)
		}
		internalSysSecretObj.Data["replication"] = pass
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

	pxcUser := "root"
	pxcPass := string(internalSysSecretObj.Data["root"])
	if _, ok := internalSysSecretObj.Data["operator"]; ok {
		pxcUser = "operator"
		pxcPass = string(internalSysSecretObj.Data["operator"])
	}

	um, err := users.NewManager(addr, pxcUser, pxcPass, cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
	if err != nil {
		return errors.Wrap(err, "new users manager")
	}
	defer um.Close()

	err = um.CreateReplicationUser(string(pass))
	if err != nil {
		return errors.Wrap(err, "create replication user")
	}

	sysUsersSecretObj.Data["replication"] = pass
	internalSysSecretObj.Data["replication"] = pass

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
