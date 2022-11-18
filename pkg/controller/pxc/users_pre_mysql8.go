package pxc

import (
	"bytes"
	"context"
	"fmt"

	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

func (r *ReconcilePerconaXtraDBCluster) updateUsersPreMYSQL8(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret) (*userUpdateActions, error) {
	res := &userUpdateActions{}

	for _, u := range users.UserNames {
		if _, ok := secrets.Data[u]; !ok {
			continue
		}

		switch u {
		case users.Root:
			if err := r.handleRootUserPreMYSQL8(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Operator:
			if err := r.handleOperatorUserPreMYSQL8(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Monitor:
			if err := r.handleMonitorUserPreMYSQL8(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Clustercheck:
			if err := r.handleClustercheckUserPreMYSQL8(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Xtrabackup:
			if err := r.handleXtrabackupUserPreMYSQL8(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Replication:
			if err := r.handleReplicationUserPreMYSQL8(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.ProxyAdmin:
			if err := r.handleProxyadminUserPreMYSQL8(cr, secrets, internalSecrets, res); err != nil {
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
func (r *ReconcilePerconaXtraDBCluster) handleRootUserPreMYSQL8(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
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

func (r *ReconcilePerconaXtraDBCluster) handleOperatorUserPreMYSQL8(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
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

func (r *ReconcilePerconaXtraDBCluster) handleMonitorUserPreMYSQL8(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
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
		mysqlVersion, err := r.mysqlVersion(cr, statefulset.NewNode(cr))
		if err != nil && !errors.Is(err, versionNotReadyErr) {
			return errors.Wrap(err, "retrieving pxc version")
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
	if cr.Spec.PMM != nil && cr.Spec.PMM.IsEnabled(internalSecrets) {
		actions.restartPXC = true
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleClustercheckUserPreMYSQL8(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
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
		mysqlVersion, err := r.mysqlVersion(cr, statefulset.NewNode(cr))
		if err != nil && !errors.Is(err, versionNotReadyErr) {
			return errors.Wrap(err, "retrieving pxc version")
		}

		if mysqlVersion != "" {
			ver, err := version.NewVersion(mysqlVersion)
			if err != nil {
				return errors.Wrap(err, "invalid pxc version")
			}

			if !ver.LessThan(privSystemUserAddedIn) {
				um, err := getUserManger(cr, internalSecrets)
				if err != nil {
					return err
				}
				defer um.Close()

				if err := r.grantSystemUserPrivilege(cr, internalSecrets, user, um); err != nil {
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

func (r *ReconcilePerconaXtraDBCluster) handleXtrabackupUserPreMYSQL8(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
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

func (r *ReconcilePerconaXtraDBCluster) handleReplicationUserPreMYSQL8(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
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

func (r *ReconcilePerconaXtraDBCluster) handleProxyadminUserPreMYSQL8(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
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
