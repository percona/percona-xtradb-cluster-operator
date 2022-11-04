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

func (r *ReconcilePerconaXtraDBCluster) updateUsers(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret) (*userUpdateActions, error) {
	res := &userUpdateActions{}

	for _, u := range users.UserNames {
		if _, ok := secrets.Data[u]; !ok {
			continue
		}

		switch u {
		case users.Root:
			if err := r.handleRootUser(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Operator:
			if err := r.handleOperatorUser(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Monitor:
			if err := r.handleMonitorUser(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Clustercheck:
			if err := r.handleClustercheckUser(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Xtrabackup:
			if err := r.handleXtrabackupUser(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Replication:
			if err := r.handleReplicationUser(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.ProxyAdmin:
			if err := r.handleProxyadminUser(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.PMMServer, users.PMMServerKey:
			if err := r.handlePMMUser(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		}
	}

	return res, nil
}

func (r *ReconcilePerconaXtraDBCluster) handleRootUser(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	logger := r.logger(cr.Name, cr.Namespace)

	user := &users.SysUser{
		Name:  users.Root,
		Pass:  string(secrets.Data[users.Root]),
		Hosts: []string{"localhost", "%"},
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	logger.Info(fmt.Sprintf("User %s: password changed, updating user", user.Name))

	err := r.updateUserPass(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update root users pass")
	}
	logger.Info(fmt.Sprintf("User %s: password updated", user.Name))

	err = r.syncPXCUsersWithProxySQL(cr)
	if err != nil {
		return errors.Wrap(err, "sync users")
	}

	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Update(context.TODO(), internalSecrets)
	if err != nil {
		return errors.Wrap(err, "update internal secrets root user password")
	}
	logger.Info(fmt.Sprintf("User %s: internal secrets updated", user.Name))

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleOperatorUser(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	logger := r.logger(cr.Name, cr.Namespace)

	if cr.Status.PXC.Ready == 0 {
		return nil
	}

	user := &users.SysUser{
		Name:  users.Operator,
		Pass:  string(secrets.Data[users.Operator]),
		Hosts: []string{"%"},
	}

	// Regardless of password change, always ensure that operator user is there with the right privileges
	err := r.manageOperatorAdminUser(cr, secrets, internalSecrets)
	if err != nil {
		return errors.Wrap(err, "manage operator admin user")
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	logger.Info(fmt.Sprintf("User %s: password changed, updating user", user.Name))

	err = r.updateUserPass(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update operator users pass")
	}
	logger.Info(fmt.Sprintf("User %s: password updated", user.Name))

	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Update(context.TODO(), internalSecrets)
	if err != nil {
		return errors.Wrap(err, "update internal users secrets operator user password")
	}
	logger.Info(fmt.Sprintf("User %s: internal secrets updated", user.Name))

	actions.restartProxy = true
	return nil
}

// manageOperatorAdminUser ensures that operator user is always present and with the right privileges
func (r *ReconcilePerconaXtraDBCluster) manageOperatorAdminUser(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret) error {
	logger := r.logger(cr.Name, cr.Namespace)

	pass, existInSys := secrets.Data[users.Operator]
	_, existInInternal := internalSecrets.Data[users.Operator]
	if existInSys && !existInInternal {
		if internalSecrets.Data == nil {
			internalSecrets.Data = make(map[string][]byte)
		}
		internalSecrets.Data[users.Operator] = pass
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
	um, err := users.NewManager(addr, users.Root, string(secrets.Data[users.Root]), cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
	if err != nil {
		return errors.Wrap(err, "new users manager")
	}
	defer um.Close()

	err = um.CreateOperatorUser(string(pass))
	if err != nil {
		return errors.Wrap(err, "create operator user")
	}

	secrets.Data[users.Operator] = pass
	internalSecrets.Data[users.Operator] = pass

	err = r.client.Update(context.TODO(), secrets)
	if err != nil {
		return errors.Wrap(err, "update sys users secret")
	}
	err = r.client.Update(context.TODO(), internalSecrets)
	if err != nil {
		return errors.Wrap(err, "update internal users secret")
	}

	logger.Info("User operator: user created and privileges granted")
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleMonitorUser(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	logger := r.logger(cr.Name, cr.Namespace)

	if cr.Status.PXC.Ready == 0 {
		return nil
	}

	user := &users.SysUser{
		Name:  users.Monitor,
		Pass:  string(secrets.Data[users.Monitor]),
		Hosts: []string{"%"},
	}

	pxcUser := users.Root
	pxcPass := string(internalSecrets.Data[users.Root])
	if _, ok := internalSecrets.Data[users.Operator]; ok {
		pxcUser = users.Operator
		pxcPass = string(internalSecrets.Data[users.Operator])
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

	// Regardless of password change, always ensure monitor user has the right privileges
	if cr.CompareVersionWith("1.6.0") >= 0 {
		err := r.updateMonitorUserGrant(cr, internalSecrets, &um)
		if err != nil {
			return errors.Wrap(err, "update monitor user grant")
		}
	}

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
				if err := r.grantSystemUserPrivilege(cr, internalSecrets, user, &um); err != nil {
					return errors.Wrap(err, "monitor user grant system privilege")
				}
			}
		}
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	logger.Info(fmt.Sprintf("User %s: password changed, updating user", user.Name))

	err = r.updateUserPass(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update monitor users pass")
	}
	logger.Info(fmt.Sprintf("User %s: password updated", user.Name))

	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		err := r.updateProxyUser(cr, internalSecrets, user)
		if err != nil {
			return errors.Wrap(err, "update monitor users pass")
		}
		logger.Info(fmt.Sprintf("User %s: proxy user updated", user.Name))
	}

	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Update(context.TODO(), internalSecrets)
	if err != nil {
		return errors.Wrap(err, "update internal users secrets monitor user password")
	}
	logger.Info(fmt.Sprintf("User %s: internal secrets updated", user.Name))

	actions.restartProxy = true
	if cr.Spec.PMM != nil && cr.Spec.PMM.Enabled {
		actions.restartPXC = true
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) updateMonitorUserGrant(cr *api.PerconaXtraDBCluster, internalSysSecretObj *corev1.Secret, um *users.Manager) error {
	logger := r.logger(cr.Name, cr.Namespace)

	annotationName := "grant-for-1.6.0-monitor-user"
	if internalSysSecretObj.Annotations[annotationName] == "done" {
		return nil
	}

	err := um.Update160MonitorUserGrant(string(internalSysSecretObj.Data["monitor"]))
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

	logger.Info("User monitor: granted privileges")
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleClustercheckUser(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	logger := r.logger(cr.Name, cr.Namespace)

	if cr.Status.PXC.Ready == 0 {
		return nil
	}

	user := &users.SysUser{
		Name:  users.Clustercheck,
		Pass:  string(secrets.Data[users.Clustercheck]),
		Hosts: []string{"localhost"},
	}

	// Regardless of password change, always ensure clustercheck user has the right privileges
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

				pxcUser := users.Root
				pxcPass := string(internalSecrets.Data[users.Root])
				if _, ok := internalSecrets.Data[users.Operator]; ok {
					pxcUser = users.Operator
					pxcPass = string(internalSecrets.Data[users.Operator])
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

				if err := r.grantSystemUserPrivilege(cr, internalSecrets, user, &um); err != nil {
					return errors.Wrap(err, "clustercheck user grant system privilege")
				}
			}
		}
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	logger.Info(fmt.Sprintf("User %s: password changed, updating user", user.Name))

	err := r.updateUserPass(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update clustercheck users pass")
	}
	logger.Info(fmt.Sprintf("User %s: password updated", user.Name))

	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Update(context.TODO(), internalSecrets)
	if err != nil {
		return errors.Wrap(err, "update internal users secrets clustercheck user password")
	}
	logger.Info(fmt.Sprintf("User %s: internal secrets updated", user.Name))

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleXtrabackupUser(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	logger := r.logger(cr.Name, cr.Namespace)

	if cr.Status.PXC.Ready == 0 {
		return nil
	}

	user := &users.SysUser{
		Name:  users.Xtrabackup,
		Pass:  string(secrets.Data[users.Xtrabackup]),
		Hosts: []string{"localhost"},
	}
	if cr.CompareVersionWith("1.7.0") >= 0 {
		user.Hosts = []string{"%"}
	}

	// Regardless of password change, always ensure xtrabackup user has the right privileges
	if cr.CompareVersionWith("1.7.0") >= 0 {
		// monitor user need more grants for work in version more then 1.6.0
		err := r.updateXtrabackupUserGrant(cr, internalSecrets)
		if err != nil {
			return errors.Wrap(err, "update xtrabackup user grant")
		}
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	logger.Info(fmt.Sprintf("User %s: password changed, updating user", user.Name))

	err := r.updateUserPass(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update xtrabackup users pass")
	}
	logger.Info(fmt.Sprintf("User %s: password updated", user.Name))

	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Update(context.TODO(), internalSecrets)
	if err != nil {
		return errors.Wrap(err, "update internal users secrets xtrabackup user password")
	}
	logger.Info(fmt.Sprintf("User %s: internal secrets updated", user.Name))

	actions.restartPXC = true
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) updateXtrabackupUserGrant(cr *api.PerconaXtraDBCluster, internalSysSecretObj *corev1.Secret) error {
	logger := r.logger(cr.Name, cr.Namespace)

	annotationName := "grant-for-1.7.0-xtrabackup-user"
	if internalSysSecretObj.Annotations[annotationName] == "done" {
		return nil
	}

	pxcUser := users.Root
	pxcPass := string(internalSysSecretObj.Data[users.Root])
	if _, ok := internalSysSecretObj.Data[users.Operator]; ok {
		pxcUser = users.Operator
		pxcPass = string(internalSysSecretObj.Data[users.Operator])
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

	err = um.Update170XtrabackupUser(string(internalSysSecretObj.Data[users.Xtrabackup]))
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

	logger.Info("User xtrabackup: granted privileges")
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleReplicationUser(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	logger := r.logger(cr.Name, cr.Namespace)

	if cr.CompareVersionWith("1.9.0") < 0 {
		return nil
	}

	if cr.Status.PXC.Ready == 0 {
		return nil
	}

	user := &users.SysUser{
		Name:  users.Replication,
		Pass:  string(secrets.Data[users.Replication]),
		Hosts: []string{"%"},
	}

	// Even if there is no password change, always ensure that operator user is there handle its grants
	err := r.manageReplicationUser(cr, secrets, internalSecrets)
	if err != nil {
		return errors.Wrap(err, "manage replication user")
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	logger.Info(fmt.Sprintf("User %s: password changed, updating user", user.Name))

	err = r.updateUserPass(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update replication users pass")
	}
	logger.Info(fmt.Sprintf("User %s: password updated", user.Name))

	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Update(context.TODO(), internalSecrets)
	if err != nil {
		return errors.Wrap(err, "update internal users secrets replication user password")
	}
	logger.Info(fmt.Sprintf("User %s: internal secrets updated", user.Name))

	actions.updateReplicationPass = true
	return nil
}

// manageReplicationUser ensures that replication user is always present and with the right privileges
func (r *ReconcilePerconaXtraDBCluster) manageReplicationUser(cr *api.PerconaXtraDBCluster, sysUsersSecretObj, internalSysSecretObj *corev1.Secret) error {
	logger := r.logger(cr.Name, cr.Namespace)

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

	logger.Info("User replication: user created and privileges granted")
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleProxyadminUser(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	logger := r.logger(cr.Name, cr.Namespace)

	if cr.Spec.ProxySQL == nil || !cr.Spec.ProxySQL.Enabled {
		return nil
	}

	user := &users.SysUser{
		Name: users.ProxyAdmin,
		Pass: string(secrets.Data[users.ProxyAdmin]),
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	logger.Info(fmt.Sprintf("User %s: password changed, updating user", user.Name))

	err := r.updateProxyUser(cr, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update Proxy users")
	}
	logger.Info(fmt.Sprintf("User %s: proxy user updated", user.Name))

	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Update(context.TODO(), internalSecrets)
	if err != nil {
		return errors.Wrap(err, "update internal users secrets proxyadmin user password")
	}
	logger.Info(fmt.Sprintf("User %s: internal secrets updated", user.Name))

	actions.restartProxy = true

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handlePMMUser(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	logger := r.logger(cr.Name, cr.Namespace)

	if cr.Spec.PMM == nil || !cr.Spec.PMM.Enabled {
		return nil
	}

	if key, ok := secrets.Data[users.PMMServerKey]; ok {
		if _, ok := internalSecrets.Data[users.PMMServerKey]; !ok {
			internalSecrets.Data[users.PMMServerKey] = key

			err := r.client.Update(context.TODO(), internalSecrets)
			if err != nil {
				return errors.Wrap(err, "update internal users secrets pmm user password")
			}
			logger.Info(fmt.Sprintf("User %s: internal secrets updated", users.PMMServerKey))

			return nil
		}
	}

	name := users.PMMServerKey
	if !cr.Spec.PMM.UseAPI(secrets) {
		name = users.PMMServer
	}

	if bytes.Equal(secrets.Data[name], internalSecrets.Data[name]) {
		return nil
	}

	logger.Info(fmt.Sprintf("User %s: password changed, updating user", name))

	internalSecrets.Data[name] = secrets.Data[name]
	err := r.client.Update(context.TODO(), internalSecrets)
	if err != nil {
		return errors.Wrap(err, "update internal users secrets pmm user password")
	}
	logger.Info(fmt.Sprintf("User %s: internal secrets updated", name))

	actions.restartPXC = true
	actions.restartProxy = true

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) syncPXCUsersWithProxySQL(cr *api.PerconaXtraDBCluster) error {
	logger := r.logger(cr.Name, cr.Namespace)

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

	logger.Info("PXC users synced with ProxySQL")
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) updateUserPass(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, user *users.SysUser) error {
	pxcUser := users.Root
	pxcPass := string(internalSecrets.Data[users.Root])
	if _, ok := secrets.Data[users.Operator]; ok {
		pxcUser = users.Operator
		pxcPass = string(internalSecrets.Data[users.Operator])
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
		um, err := users.NewManager(cr.Name+"-proxysql-"+strconv.Itoa(i)+"."+cr.Name+"-proxysql-unready."+cr.Namespace+":6032", users.ProxyAdmin, string(internalSecrets.Data[users.ProxyAdmin]), cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
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

func (r *ReconcilePerconaXtraDBCluster) grantSystemUserPrivilege(cr *api.PerconaXtraDBCluster, internalSysSecretObj *corev1.Secret, user *users.SysUser, um *users.Manager) error {
	logger := r.logger(cr.Name, cr.Namespace)

	annotationName := "grant-for-1.10.0-system-privilege"
	if internalSysSecretObj.Annotations[annotationName] == "done" {
		return nil
	}

	if err := um.Update1100SystemUserPrivilege(user); err != nil {
		return errors.Wrap(err, "grant system user privilege")
	}

	if internalSysSecretObj.Annotations == nil {
		internalSysSecretObj.Annotations = make(map[string]string)
	}

	internalSysSecretObj.Annotations[annotationName] = "done"
	err := r.client.Update(context.TODO(), internalSysSecretObj)
	if err != nil {
		return errors.Wrap(err, "update internal sys users secret annotation")
	}

	logger.Info(fmt.Sprintf("User %s: system user privileges granted", user.Name))
	return nil
}
