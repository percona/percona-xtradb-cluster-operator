package pxc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strconv"

	"github.com/hashicorp/go-version"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const internalSecretsPrefix = "internal-"

// https://dev.mysql.com/doc/refman/8.0/en/privileges-provided.html#priv_system-user
var privSystemUserAddedIn = version.Must(version.NewVersion("8.0.16"))

type userUpdateActions struct {
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
	secrets := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.SecretsName,
		},
		&secrets,
	)
	if err != nil && k8serrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Wrapf(err, "get sys users secret '%s'", cr.Spec.SecretsName)
	}

	internalSecretName := internalSecretsPrefix + cr.Name

	internalSecrets := corev1.Secret{}
	err = r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      internalSecretName,
		},
		&internalSecrets,
	)
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, errors.Wrap(err, "get internal sys users secret")
	}

	if k8serrors.IsNotFound(err) {
		is := secrets.DeepCopy()
		is.ObjectMeta = metav1.ObjectMeta{
			Name:      internalSecretName,
			Namespace: cr.Namespace,
		}
		err = r.client.Create(context.TODO(), is)
		if err != nil {
			return nil, errors.Wrap(err, "create internal sys users secret")
		}
		return nil, nil
	}

	if reflect.DeepEqual(secrets.Data, internalSecrets.Data) {
		log.Printf("user secrets not changed.\n")
		return nil, nil
	}

	if cr.Status.Status != api.AppStateReady {
		return nil, nil
	}

	actions, err := r.updateUsers(cr, &secrets, &internalSecrets)
	if err != nil {
		return nil, errors.Wrap(err, "manage sys users")
	}

	newSysData, err := json.Marshal(secrets.Data)
	if err != nil {
		return nil, errors.Wrap(err, "marshal sys secret data")
	}
	newSecretDataHash := sha256Hash(newSysData)

	result := &ReconcileUsersResult{
		updateReplicationPassword: actions.updateReplicationPass,
	}

	if actions.restartProxy {
		result.proxysqlAnnotations = map[string]string{"last-applied-secret": newSecretDataHash}
	}
	if actions.restartPXC {
		result.pxcAnnotations = map[string]string{"last-applied-secret": newSecretDataHash}
	}

	return result, nil
}

func sha256Hash(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func (r *ReconcilePerconaXtraDBCluster) updateUsers(
	cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret) (*userUpdateActions, error) {
	res := &userUpdateActions{}

	for _, u := range users.UserNames {
		if _, ok := secrets.Data[u]; !ok ||
			bytes.Equal(secrets.Data[u], internalSecrets.Data[u]) {
			continue
		}

		switch u {
		case users.UserRoot:
			if err := r.handleRootUser(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.UserOperator:
			if err := r.handleOperatorUser(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.UserMonitor:
			if err := r.handleMonitorUser(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.UserClustercheck:
			if err := r.handleClustercheckUser(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.UserXtrabackup:
			if err := r.handleXtrabackupUser(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.UserReplication:
			if err := r.handleReplicationUser(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.UserProxyAdmin:
			if err := r.handleProxyadminUser(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.UserPMMServer, users.UserPMMServerKey:
			if err := r.handlePMMUser(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		}
	}

	return res, nil
}

func (r *ReconcilePerconaXtraDBCluster) handleRootUser(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	user := &users.SysUser{
		Name:  users.UserRoot,
		Pass:  string(secrets.Data[users.UserRoot]),
		Hosts: []string{"localhost", "%"},
	}

	// update pass
	err := r.updateUserPass(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update root users pass")
	}

	// syncProxyUser
	err = r.syncPXCUsersWithProxySQL(cr)
	if err != nil {
		return errors.Wrap(err, "sync users")
	}

	//update internalSecrets
	internalSecrets.Data[users.UserRoot] = secrets.Data[users.UserRoot]
	err = r.client.Update(context.TODO(), internalSecrets)
	if err != nil {
		return errors.Wrap(err, "update internal secrets root user password")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleOperatorUser(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	user := &users.SysUser{
		Name:  users.UserOperator,
		Pass:  string(secrets.Data[users.UserOperator]),
		Hosts: []string{"localhost", "%"},
	}

	err := r.manageOperatorAdminUser(cr, secrets, internalSecrets)
	if err != nil {
		return errors.Wrap(err, "manage operator admin user")
	}

	// update pass
	err = r.updateUserPass(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update operator users pass")
	}

	//update internalSecrets
	internalSecrets.Data[users.UserRoot] = secrets.Data[users.UserRoot]
	err = r.client.Update(context.TODO(), internalSecrets)
	if err != nil {
		return errors.Wrap(err, "update internal users secrets operator user password")
	}

	actions.restartPXC = true
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) manageOperatorAdminUser(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret) error {
	pass, existInSys := secrets.Data[users.UserOperator]
	_, existInInternal := internalSecrets.Data[users.UserOperator]
	if existInSys && !existInInternal {
		if internalSecrets.Data == nil {
			internalSecrets.Data = make(map[string][]byte)
		}
		internalSecrets.Data[users.UserOperator] = pass
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
	um, err := users.NewManager(addr, users.UserRoot, string(secrets.Data[users.UserRoot]), cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
	if err != nil {
		return errors.Wrap(err, "new users manager")
	}
	defer um.Close()

	err = um.CreateOperatorUser(string(pass))
	if err != nil {
		return errors.Wrap(err, "create operator user")
	}

	secrets.Data[users.UserOperator] = pass
	internalSecrets.Data[users.UserOperator] = pass

	err = r.client.Update(context.TODO(), secrets)
	if err != nil {
		return errors.Wrap(err, "update sys users secret")
	}
	err = r.client.Update(context.TODO(), internalSecrets)
	if err != nil {
		return errors.Wrap(err, "update internal users secret")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleMonitorUser(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	user := &users.SysUser{
		Name:  users.UserMonitor,
		Pass:  string(secrets.Data[users.UserMonitor]),
		Hosts: []string{"%"},
	}

	//update grants
	if cr.CompareVersionWith("1.6.0") >= 0 {
		// monitor user need more grants for work in version more then 1.6.0
		err := r.updateMonitorUserGrant(cr, internalSecrets)
		if err != nil {
			return errors.Wrap(err, "update monitor user grant")
		}
	}

	// grant system user privilege
	if cr.CompareVersionWith("1.10.0") >= 0 {
		mysqlVersion := cr.Status.PXC.Version
		if mysqlVersion == "" {
			var err error
			mysqlVersion, err = r.mysqlVersion(cr, statefulset.NewNode(cr))
			if err != nil && !errors.Is(err, versionNotReadyErr) {
				return errors.Wrap(err, "retrieving pxc version")
			}
		}

		if mysqlVersion != "" {
			ver, err := version.NewVersion(mysqlVersion)
			if err != nil {
				return errors.Wrap(err, "invalid pxc version")
			}

			if !ver.LessThan(privSystemUserAddedIn) {
				if err := r.grantSystemUserPrivilege(cr, internalSecrets, user); err != nil {
					return errors.Wrap(err, "monitor user grant system privilege")
				}
			}
		}
	}

	// update proxy users
	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		err := r.updateProxyUser(cr, internalSecrets, user)
		if err != nil {
			return errors.Wrap(err, "update monitor users pass")
		}
	}

	//update internalSecrets
	internalSecrets.Data[users.UserRoot] = secrets.Data[users.UserRoot]
	err := r.client.Update(context.TODO(), internalSecrets)
	if err != nil {
		return errors.Wrap(err, "update internal users secrets monitor user password")
	}

	actions.restartProxy = true
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) updateMonitorUserGrant(cr *api.PerconaXtraDBCluster, internalSysSecretObj *corev1.Secret) error {
	// Q: Why do we need these done annotations? For other users as well?
	annotationName := "grant-for-1.6.0-monitor-user"
	if internalSysSecretObj.Annotations[annotationName] == "done" {
		return nil
	}

	pxcUser := users.UserRoot
	pxcPass := string(internalSysSecretObj.Data[users.UserRoot])
	if _, ok := internalSysSecretObj.Data[users.UserOperator]; ok {
		pxcUser = users.UserOperator
		pxcPass = string(internalSysSecretObj.Data[users.UserOperator])
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

func (r *ReconcilePerconaXtraDBCluster) handleClustercheckUser(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	user := &users.SysUser{
		Name:  users.UserClustercheck,
		Pass:  string(secrets.Data[users.UserClustercheck]),
		Hosts: []string{"localhost"},
	}

	// grant system user privilege
	if cr.CompareVersionWith("1.10.0") >= 0 {
		mysqlVersion := cr.Status.PXC.Version
		if mysqlVersion == "" {
			var err error
			mysqlVersion, err = r.mysqlVersion(cr, statefulset.NewNode(cr))
			if err != nil && !errors.Is(err, versionNotReadyErr) {
				return errors.Wrap(err, "retrieving pxc version")
			}
		}

		if mysqlVersion != "" {
			ver, err := version.NewVersion(mysqlVersion)
			if err != nil {
				return errors.Wrap(err, "invalid pxc version")
			}

			if !ver.LessThan(privSystemUserAddedIn) {
				if err := r.grantSystemUserPrivilege(cr, internalSecrets, user); err != nil {
					return errors.Wrap(err, "clustercheck user grant system privilege")
				}
			}
		}
	}

	// update pass
	err := r.updateUserPass(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update clustercheck users pass")
	}

	//update internalSecrets
	internalSecrets.Data[users.UserRoot] = secrets.Data[users.UserRoot]
	err = r.client.Update(context.TODO(), internalSecrets)
	if err != nil {
		return errors.Wrap(err, "update internal users secrets clustercheck user password")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleXtrabackupUser(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	user := &users.SysUser{
		Name:  users.UserXtrabackup,
		Pass:  string(secrets.Data[users.UserXtrabackup]),
		Hosts: []string{"localhost"},
	}
	if cr.CompareVersionWith("1.7.0") >= 0 {
		user.Hosts = []string{"%"}
	}

	//update grants
	if cr.CompareVersionWith("1.7.0") >= 0 {
		// monitor user need more grants for work in version more then 1.6.0
		err := r.updateXtrabackupUserGrant(cr, internalSecrets)
		if err != nil {
			return errors.Wrap(err, "update xtrabackup user grant")
		}
	}

	// update pass
	err := r.updateUserPass(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update xtrabackup users pass")
	}

	//update internalSecrets
	internalSecrets.Data[users.UserRoot] = secrets.Data[users.UserRoot]
	err = r.client.Update(context.TODO(), internalSecrets)
	if err != nil {
		return errors.Wrap(err, "update internal users secrets xtrabackup user password")
	}

	actions.restartPXC = true
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) updateXtrabackupUserGrant(cr *api.PerconaXtraDBCluster, internalSysSecretObj *corev1.Secret) error {
	annotationName := "grant-for-1.7.0-xtrabackup-user"
	if internalSysSecretObj.Annotations[annotationName] == "done" {
		return nil
	}

	pxcUser := users.UserRoot
	pxcPass := string(internalSysSecretObj.Data[users.UserRoot])
	if _, ok := internalSysSecretObj.Data[users.UserOperator]; ok {
		pxcUser = users.UserOperator
		pxcPass = string(internalSysSecretObj.Data[users.UserOperator])
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

func (r *ReconcilePerconaXtraDBCluster) handleReplicationUser(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	if cr.CompareVersionWith("1.9.0") >= 0 {
		return errors.New("CR version 1.9.0 requered")
	}

	user := &users.SysUser{
		Name:  users.UserReplication,
		Pass:  string(secrets.Data[users.UserReplication]),
		Hosts: []string{"%"},
	}

	err := r.manageReplicationUser(cr, secrets, internalSecrets)
	if err != nil {
		return errors.Wrap(err, "manage replication user")
	}

	// update pass
	err = r.updateUserPass(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update replication users pass")
	}

	//update internalSecrets
	internalSecrets.Data[users.UserRoot] = secrets.Data[users.UserRoot]
	err = r.client.Update(context.TODO(), internalSecrets)
	if err != nil {
		return errors.Wrap(err, "update internal users secrets replication user password")
	}

	actions.updateReplicationPass = true
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

func (r *ReconcilePerconaXtraDBCluster) handleProxyadminUser(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		return errors.New("ProxySQL not enabled")
	}

	user := &users.SysUser{
		Name: users.UserProxyAdmin,
		Pass: string(secrets.Data[users.UserProxyAdmin]),
	}

	// update proxy users
	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		err := r.updateProxyUser(cr, internalSecrets, user)
		if err != nil {
			return errors.Wrap(err, "update Proxy users")
		}
	}

	// syncProxyUser
	err := r.syncPXCUsersWithProxySQL(cr)
	if err != nil {
		return errors.Wrap(err, "sync proxy users")
	}

	//update internalSecrets
	internalSecrets.Data[users.UserRoot] = secrets.Data[users.UserRoot]
	err = r.client.Update(context.TODO(), internalSecrets)
	if err != nil {
		return errors.Wrap(err, "update internal users secrets proxyadmin user password")
	}

	actions.restartProxy = true
	if cr.Spec.PMM != nil && cr.Spec.PMM.Enabled {
		actions.restartPXC = true
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handlePMMUser(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	if cr.Spec.PMM != nil && cr.Spec.PMM.Enabled {
		return errors.New("PMM not enabled")
	}

	//update internalSecrets
	internalSecrets.Data[users.UserRoot] = secrets.Data[users.UserRoot]
	err := r.client.Update(context.TODO(), internalSecrets)
	if err != nil {
		return errors.Wrap(err, "update internal users secrets pmm user password")
	}

	actions.restartPXC = true
	actions.restartProxy = true

	return nil
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
		err = r.clientcmd.Exec(&pod, "proxysql", []string{"proxysql-admin", "--syncusers", "--add-query-rule"}, nil, &outb, &errb, false)
		if err != nil {
			return errors.Errorf("exec syncusers: %v / %s / %s", err, outb.String(), errb.String())
		}
		if len(errb.Bytes()) > 0 {
			return errors.New("syncusers: " + errb.String())
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) updateUserPass(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, user *users.SysUser) error {
	pxcUser := users.UserRoot
	pxcPass := string(internalSecrets.Data[users.UserRoot])
	if _, ok := secrets.Data[users.UserOperator]; ok {
		pxcUser = users.UserOperator
		pxcPass = string(internalSecrets.Data[users.UserOperator])
	}

	addr := cr.Name + "-pxc." + cr.Namespace
	if cr.CompareVersionWith("1.6.0") >= 0 {
		addr = cr.Name + "-pxc-unready." + cr.Namespace + ":33062"
	}
	um, err := users.NewManager(addr, pxcUser, pxcPass, cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
	if err != nil {
		return errors.Wrap(err, "new users manager")
	}
	defer um.Close()

	err = um.UpdateUserPass(user)
	if err != nil {
		return errors.Wrap(err, "update user pass")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) updateProxyUser(cr *api.PerconaXtraDBCluster, internalSecrets *corev1.Secret, user *users.SysUser) error {
	if user == nil {
		return nil
	}

	for i := 0; i < int(cr.Spec.ProxySQL.Size); i++ {
		um, err := users.NewManager(cr.Name+"-proxysql-"+strconv.Itoa(i)+"."+cr.Name+"-proxysql-unready."+cr.Namespace+":6032", "proxyadmin", string(internalSecrets.Data["proxyadmin"]), cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
		if err != nil {
			return errors.Wrap(err, "new users manager")
		}
		defer um.Close()
		err = um.UpdateProxyUser(user)
		if err != nil {
			return errors.Wrap(err, "update proxy users")
		}
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) grantSystemUserPrivilege(cr *api.PerconaXtraDBCluster, internalSysSecretObj *corev1.Secret, user *users.SysUser) error {
	annotationName := "grant-for-1.10.0-system-privilege"
	if internalSysSecretObj.Annotations[annotationName] == "done" {
		return nil
	}

	pxcUser := users.UserRoot
	pxcPass := string(internalSysSecretObj.Data[users.UserRoot])
	if _, ok := internalSysSecretObj.Data[users.UserOperator]; ok {
		pxcUser = users.UserOperator
		pxcPass = string(internalSysSecretObj.Data[users.UserOperator])
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

	if err = um.Update1100SystemUserPrivilege(user); err != nil {
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
