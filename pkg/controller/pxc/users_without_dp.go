package pxc

import (
	"bytes"
	"context"
	"fmt"

	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

func (r *ReconcilePerconaXtraDBCluster) updateUsersWithoutDP(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret) (*userUpdateActions, error) {
	res := &userUpdateActions{}

	for _, u := range users.UserNames {
		if _, ok := secrets.Data[u]; !ok {
			continue
		}

		switch u {
		case users.Root:
			if err := r.handleRootUserWithoutDP(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Operator:
			if err := r.handleOperatorUserWithoutDP(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Monitor:
			if err := r.handleMonitorUserWithoutDP(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Clustercheck:
			if err := r.handleClustercheckUserWithoutDP(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Xtrabackup:
			if err := r.handleXtrabackupUserWithoutDP(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Replication:
			if err := r.handleReplicationUserWithoutDP(cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.ProxyAdmin:
			if err := r.handleProxyadminUserWithoutDP(cr, secrets, internalSecrets, res); err != nil {
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
func (r *ReconcilePerconaXtraDBCluster) handleRootUserWithoutDP(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	if cr.Status.Status != api.AppStateReady {
		return nil
	}

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

	err := r.updateUserPassWithoutDP(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update root users pass")
	}
	logger.Info(fmt.Sprintf("User %s: password updated", user.Name))

	err = r.syncPXCUsersWithProxySQL(cr)
	if err != nil {
		return errors.Wrap(err, "sync users")
	}

	orig := internalSecrets.DeepCopy()
	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Patch(context.TODO(), internalSecrets, client.MergeFrom(orig))
	if err != nil {
		return errors.Wrap(err, "update internal secrets root user password")
	}
	logger.Info(fmt.Sprintf("User %s: internal secrets updated", user.Name))

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleOperatorUserWithoutDP(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	logger := r.logger(cr.Name, cr.Namespace)

	user := &users.SysUser{
		Name:  users.Operator,
		Pass:  string(secrets.Data[users.Operator]),
		Hosts: []string{"%"},
	}

	if cr.Status.PXC.Ready > 0 {
		err := r.manageOperatorAdminUser(cr, secrets, internalSecrets)
		if err != nil {
			return errors.Wrap(err, "manage operator admin user")
		}
	}

	if cr.Status.Status != api.AppStateReady {
		return nil
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	logger.Info(fmt.Sprintf("User %s: password changed, updating user", user.Name))

	err := r.updateUserPassWithoutDP(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update operator users pass")
	}
	logger.Info(fmt.Sprintf("User %s: password updated", user.Name))

	orig := internalSecrets.DeepCopy()
	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Patch(context.TODO(), internalSecrets, client.MergeFrom(orig))
	if err != nil {
		return errors.Wrap(err, "update internal users secrets operator user password")
	}
	logger.Info(fmt.Sprintf("User %s: internal secrets updated", user.Name))

	actions.restartProxy = true
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleMonitorUserWithoutDP(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	logger := r.logger(cr.Name, cr.Namespace)

	user := &users.SysUser{
		Name:  users.Monitor,
		Pass:  string(secrets.Data[users.Monitor]),
		Hosts: []string{"%"},
	}

	if cr.Status.PXC.Ready > 0 {
		um, err := getUserManager(cr, internalSecrets)
		if err != nil {
			return err
		}
		defer um.Close()

		if cr.CompareVersionWith("1.6.0") >= 0 {
			err := r.updateMonitorUserGrant(cr, internalSecrets, um)
			if err != nil {
				return errors.Wrap(err, "update monitor user grant")
			}
		}

		if cr.CompareVersionWith("1.10.0") >= 0 {
			mysqlVersion := cr.Status.PXC.Version
			if mysqlVersion == "" {
				var err error
				mysqlVersion, err = r.mysqlVersion(cr, statefulset.NewNode(cr))
				if err != nil {
					if errors.Is(err, versionNotReadyErr) {
						return nil
					}
					return errors.Wrap(err, "retrieving pxc version")
				}
			}

			if mysqlVersion != "" {
				ver, err := version.NewVersion(mysqlVersion)
				if err != nil {
					return errors.Wrap(err, "invalid pxc version")
				}

				if !ver.LessThan(privSystemUserAddedIn) {
					if err := r.grantSystemUserPrivilege(cr, internalSecrets, user, um); err != nil {
						return errors.Wrap(err, "monitor user grant system privilege")
					}
				}
			}
		}
	}

	if cr.Status.Status != api.AppStateReady {
		return nil
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	logger.Info(fmt.Sprintf("User %s: password changed, updating user", user.Name))

	err := r.updateUserPassWithoutDP(cr, secrets, internalSecrets, user)
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

	actions.restartProxy = true
	if cr.Spec.PMM != nil && cr.Spec.PMM.IsEnabled(internalSecrets) {
		actions.restartPXC = true
	}

	orig := internalSecrets.DeepCopy()
	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Patch(context.TODO(), internalSecrets, client.MergeFrom(orig))
	if err != nil {
		return errors.Wrap(err, "update internal users secrets monitor user password")
	}
	logger.Info(fmt.Sprintf("User %s: internal secrets updated", user.Name))

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleClustercheckUserWithoutDP(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	logger := r.logger(cr.Name, cr.Namespace)

	user := &users.SysUser{
		Name:  users.Clustercheck,
		Pass:  string(secrets.Data[users.Clustercheck]),
		Hosts: []string{"localhost"},
	}

	if cr.Status.PXC.Ready > 0 {
		if cr.CompareVersionWith("1.10.0") >= 0 {
			mysqlVersion := cr.Status.PXC.Version
			if mysqlVersion == "" {
				var err error
				mysqlVersion, err = r.mysqlVersion(cr, statefulset.NewNode(cr))
				if err != nil {
					if errors.Is(err, versionNotReadyErr) {
						return nil
					}
					return errors.Wrap(err, "retrieving pxc version")
				}
			}

			if mysqlVersion != "" {
				ver, err := version.NewVersion(mysqlVersion)
				if err != nil {
					return errors.Wrap(err, "invalid pxc version")
				}

				if !ver.LessThan(privSystemUserAddedIn) {
					um, err := getUserManager(cr, internalSecrets)
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
	}

	if cr.Status.Status != api.AppStateReady {
		return nil
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	logger.Info(fmt.Sprintf("User %s: password changed, updating user", user.Name))

	err := r.updateUserPassWithoutDP(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update clustercheck users pass")
	}
	logger.Info(fmt.Sprintf("User %s: password updated", user.Name))

	orig := internalSecrets.DeepCopy()
	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Patch(context.TODO(), internalSecrets, client.MergeFrom(orig))
	if err != nil {
		return errors.Wrap(err, "update internal users secrets clustercheck user password")
	}
	logger.Info(fmt.Sprintf("User %s: internal secrets updated", user.Name))

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleXtrabackupUserWithoutDP(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	logger := r.logger(cr.Name, cr.Namespace)

	user := &users.SysUser{
		Name:  users.Xtrabackup,
		Pass:  string(secrets.Data[users.Xtrabackup]),
		Hosts: []string{"localhost"},
	}

	if cr.CompareVersionWith("1.7.0") >= 0 {
		user.Hosts = []string{"%"}
	}

	if cr.Status.PXC.Ready > 0 {
		if cr.CompareVersionWith("1.7.0") >= 0 {
			// monitor user need more grants for work in version more then 1.6.0
			err := r.updateXtrabackupUserGrant(cr, internalSecrets)
			if err != nil {
				return errors.Wrap(err, "update xtrabackup user grant")
			}
		}
	}

	if cr.Status.Status != api.AppStateReady {
		return nil
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	logger.Info(fmt.Sprintf("User %s: password changed, updating user", user.Name))

	err := r.updateUserPassWithoutDP(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update xtrabackup users pass")
	}
	logger.Info(fmt.Sprintf("User %s: password updated", user.Name))

	orig := internalSecrets.DeepCopy()
	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Patch(context.TODO(), internalSecrets, client.MergeFrom(orig))
	if err != nil {
		return errors.Wrap(err, "update internal users secrets xtrabackup user password")
	}
	logger.Info(fmt.Sprintf("User %s: internal secrets updated", user.Name))

	actions.restartPXC = true
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleReplicationUserWithoutDP(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	logger := r.logger(cr.Name, cr.Namespace)

	if cr.CompareVersionWith("1.9.0") < 0 {
		return nil
	}

	if cr.Status.Status != api.AppStateReady {
		return nil
	}

	user := &users.SysUser{
		Name:  users.Replication,
		Pass:  string(secrets.Data[users.Replication]),
		Hosts: []string{"%"},
	}

	if cr.Status.PXC.Ready > 0 {
		err := r.manageReplicationUser(cr, secrets, internalSecrets)
		if err != nil {
			return errors.Wrap(err, "manage replication user")
		}
	}

	if cr.Status.Status != api.AppStateReady {
		return nil
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	logger.Info(fmt.Sprintf("User %s: password changed, updating user", user.Name))

	err := r.updateUserPassWithoutDP(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update replication users pass")
	}
	logger.Info(fmt.Sprintf("User %s: password updated", user.Name))

	orig := internalSecrets.DeepCopy()
	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Patch(context.TODO(), internalSecrets, client.MergeFrom(orig))
	if err != nil {
		return errors.Wrap(err, "update internal users secrets replication user password")
	}
	logger.Info(fmt.Sprintf("User %s: internal secrets updated", user.Name))

	actions.updateReplicationPass = true
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleProxyadminUserWithoutDP(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
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

	if cr.Status.Status != api.AppStateReady {
		return nil
	}

	logger.Info(fmt.Sprintf("User %s: password changed, updating user", user.Name))

	err := r.updateProxyUser(cr, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update Proxy users")
	}
	logger.Info(fmt.Sprintf("User %s: proxy user updated", user.Name))

	orig := internalSecrets.DeepCopy()
	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Patch(context.TODO(), internalSecrets, client.MergeFrom(orig))
	if err != nil {
		return errors.Wrap(err, "update internal users secrets proxyadmin user password")
	}
	logger.Info(fmt.Sprintf("User %s: internal secrets updated", user.Name))

	actions.restartProxy = true

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) updateUserPassWithoutDP(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, user *users.SysUser) error {
	um, err := getUserManager(cr, internalSecrets)
	if err != nil {
		return err
	}
	defer um.Close()

	err = um.UpdateUserPassWithoutDP(user)
	if err != nil {
		return errors.Wrap(err, "update user pass")
	}

	return nil
}
