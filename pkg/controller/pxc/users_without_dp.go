package pxc

import (
	"bytes"
	"context"

	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

func (r *ReconcilePerconaXtraDBCluster) updateUsersWithoutDP(ctx context.Context, cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret) (*userUpdateActions, error) {
	res := &userUpdateActions{}

	for _, u := range users.UserNames {
		if _, ok := secrets.Data[u]; !ok {
			continue
		}

		switch u {
		case users.Root:
			if err := r.handleRootUserWithoutDP(ctx, cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Operator:
			if err := r.handleOperatorUserWithoutDP(ctx, cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Monitor:
			if err := r.handleMonitorUserWithoutDP(ctx, cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Xtrabackup:
			if err := r.handleXtrabackupUserWithoutDP(ctx, cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Replication:
			if err := r.handleReplicationUserWithoutDP(ctx, cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.ProxyAdmin:
			if err := r.handleProxyadminUserWithoutDP(ctx, cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.PMMServer, users.PMMServerKey:
			if err := r.handlePMMUser(ctx, cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.PMMServerToken:
			if err := r.handlePMM3User(ctx, cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		}
	}

	return res, nil
}

func (r *ReconcilePerconaXtraDBCluster) handleRootUserWithoutDP(ctx context.Context, cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	if cr.Status.Status != api.AppStateReady && !r.invalidPasswordApplied(cr.Status) {
		return nil
	}

	log := logf.FromContext(ctx)

	user := &users.SysUser{
		Name:  users.Root,
		Pass:  string(secrets.Data[users.Root]),
		Hosts: []string{"localhost", "%"},
	}

	if err := r.updateUserPassExpirationPolicy(ctx, cr, internalSecrets, user); err != nil {
		return err
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	log.Info("Password changed, updating user", "user", user.Name)

	err := r.updateUserPassWithoutDP(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update root users pass")
	}
	log.Info("User password updated", "user", user.Name)

	if err := r.updateMySQLInitFile(ctx, cr, internalSecrets, user); err != nil {
		return errors.Wrap(err, "update mysql init file")
	}

	err = r.syncPXCUsersWithProxySQL(ctx, cr)
	if err != nil {
		return errors.Wrap(err, "sync users")
	}

	orig := internalSecrets.DeepCopy()
	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Patch(context.TODO(), internalSecrets, client.MergeFrom(orig))
	if err != nil {
		return errors.Wrap(err, "update internal secrets root user password")
	}
	log.Info("Internal secrets updated", "user", user.Name)

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleOperatorUserWithoutDP(ctx context.Context, cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	log := logf.FromContext(ctx)

	user := &users.SysUser{
		Name:  users.Operator,
		Pass:  string(secrets.Data[users.Operator]),
		Hosts: []string{"%"},
	}

	if cr.Status.PXC.Ready > 0 {
		err := r.manageOperatorAdminUser(ctx, cr, secrets, internalSecrets)
		if err != nil {
			return errors.Wrap(err, "manage operator admin user")
		}

		if err := r.updateUserPassExpirationPolicy(ctx, cr, internalSecrets, user); err != nil {
			return err
		}
	}

	if cr.Status.Status != api.AppStateReady && !r.invalidPasswordApplied(cr.Status) {
		return nil
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	log.Info("Password changed, updating user", "user", user.Name)

	err := r.updateUserPassWithoutDP(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update operator users pass")
	}
	log.Info("User password updated", "user", user.Name)

	if err := r.updateMySQLInitFile(ctx, cr, internalSecrets, user); err != nil {
		return errors.Wrap(err, "update mysql init file")
	}

	orig := internalSecrets.DeepCopy()
	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Patch(context.TODO(), internalSecrets, client.MergeFrom(orig))
	if err != nil {
		return errors.Wrap(err, "update internal users secrets operator user password")
	}
	log.Info("Internal secrets updated", "user", user.Name)

	actions.restartProxySQL = true
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleMonitorUserWithoutDP(ctx context.Context, cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	log := logf.FromContext(ctx)

	user := &users.SysUser{
		Name:  users.Monitor,
		Pass:  string(secrets.Data[users.Monitor]),
		Hosts: []string{"%"},
	}

	if cr.Status.PXC.Ready > 0 {
		if err := r.updateUserPassExpirationPolicy(ctx, cr, internalSecrets, user); err != nil {
			return err
		}

		um, err := getUserManager(cr, internalSecrets)
		if err != nil {
			return err
		}
		defer um.Close()

		if cr.CompareVersionWith("1.6.0") >= 0 {
			err := r.updateMonitorUserGrant(ctx, cr, internalSecrets, um)
			if err != nil {
				return errors.Wrap(err, "update monitor user grant")
			}
		}

		if cr.CompareVersionWith("1.10.0") >= 0 {
			mysqlVersion := cr.Status.PXC.Version
			if mysqlVersion == "" {
				var err error
				mysqlVersion, err = r.mysqlVersion(ctx, cr, statefulset.NewNode(cr))
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
					if err := r.grantMonitorUserPrivilege(ctx, cr, internalSecrets, um); err != nil {
						return errors.Wrap(err, "monitor user grant system privilege")
					}
				}
			}
		}
	}

	if cr.Status.Status != api.AppStateReady && !r.invalidPasswordApplied(cr.Status) {
		return nil
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	log.Info("Password changed, updating user", "user", user.Name)

	err := r.updateUserPassWithoutDP(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update monitor users pass")
	}
	log.Info("User password updated", "user", user.Name)

	if err := r.updateMySQLInitFile(ctx, cr, internalSecrets, user); err != nil {
		return errors.Wrap(err, "update mysql init file")
	}

	if cr.Spec.ProxySQLEnabled() {
		err := r.updateProxyUser(cr, internalSecrets, user)
		if err != nil {
			return errors.Wrap(err, "update monitor users pass")
		}
		log.Info("Proxy user updated", "user", user.Name)
	}

	// We should restart HAProxy if the monitor user password has been changed only on version 5.7
	actions.restartHAProxy = true

	actions.restartProxySQL = true
	if cr.Spec.PMM != nil && cr.Spec.PMM.IsEnabled(internalSecrets) {
		actions.restartPXC = true
	}
	if cr.Spec.PXC.Sidecars != nil && cr.Spec.PXC.HasSidecarInternalSecret(internalSecrets) {
		actions.restartPXC = true
	}

	orig := internalSecrets.DeepCopy()
	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Patch(context.TODO(), internalSecrets, client.MergeFrom(orig))
	if err != nil {
		return errors.Wrap(err, "update internal users secrets monitor user password")
	}
	log.Info("Internal secrets updated", "user", user.Name)

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleXtrabackupUserWithoutDP(ctx context.Context, cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	log := logf.FromContext(ctx)

	user := &users.SysUser{
		Name:  users.Xtrabackup,
		Pass:  string(secrets.Data[users.Xtrabackup]),
		Hosts: []string{"%"},
	}

	if cr.Status.PXC.Ready > 0 {
		if err := r.updateUserPassExpirationPolicy(ctx, cr, internalSecrets, user); err != nil {
			return err
		}

		if cr.CompareVersionWith("1.15.0") >= 0 {
			err := r.updateXtrabackupUserGrant(ctx, cr, internalSecrets)
			if err != nil {
				return errors.Wrap(err, "update xtrabackup user grant")
			}
		}
	}

	if cr.Status.Status != api.AppStateReady && !r.invalidPasswordApplied(cr.Status) {
		return nil
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	log.Info("Password changed, updating user", "user", user.Name)

	err := r.updateUserPassWithoutDP(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update xtrabackup users pass")
	}
	log.Info("User password updated", "user", user.Name)

	if err := r.updateMySQLInitFile(ctx, cr, internalSecrets, user); err != nil {
		return errors.Wrap(err, "update mysql init file")
	}

	orig := internalSecrets.DeepCopy()
	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Patch(context.TODO(), internalSecrets, client.MergeFrom(orig))
	if err != nil {
		return errors.Wrap(err, "update internal users secrets xtrabackup user password")
	}
	log.Info("Internal secrets updated", "user", user.Name)

	actions.restartPXC = true
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleReplicationUserWithoutDP(ctx context.Context, cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	log := logf.FromContext(ctx)

	if cr.CompareVersionWith("1.9.0") < 0 {
		return nil
	}

	if cr.Status.Status != api.AppStateReady && !r.invalidPasswordApplied(cr.Status) {
		return nil
	}

	user := &users.SysUser{
		Name:  users.Replication,
		Pass:  string(secrets.Data[users.Replication]),
		Hosts: []string{"%"},
	}

	if cr.Status.PXC.Ready > 0 {
		err := r.manageReplicationUser(ctx, cr, secrets, internalSecrets)
		if err != nil {
			return errors.Wrap(err, "manage replication user")
		}

		if err := r.updateUserPassExpirationPolicy(ctx, cr, internalSecrets, user); err != nil {
			return err
		}
	}

	if cr.Status.Status != api.AppStateReady && !r.invalidPasswordApplied(cr.Status) {
		return nil
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	log.Info("Password changed, updating user", "user", user.Name)

	err := r.updateUserPassWithoutDP(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update replication users pass")
	}
	log.Info("User password updated", "user", user.Name)

	if err := r.updateMySQLInitFile(ctx, cr, internalSecrets, user); err != nil {
		return errors.Wrap(err, "update mysql init file")
	}

	orig := internalSecrets.DeepCopy()
	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Patch(context.TODO(), internalSecrets, client.MergeFrom(orig))
	if err != nil {
		return errors.Wrap(err, "update internal users secrets replication user password")
	}
	log.Info("Internal secrets updated", "user", user.Name)

	actions.updateReplicationPass = true
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleProxyadminUserWithoutDP(ctx context.Context, cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	log := logf.FromContext(ctx)

	if !cr.Spec.ProxySQLEnabled() {
		return nil
	}

	user := &users.SysUser{
		Name: users.ProxyAdmin,
		Pass: string(secrets.Data[users.ProxyAdmin]),
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	if cr.Status.Status != api.AppStateReady && !r.invalidPasswordApplied(cr.Status) {
		return nil
	}

	if err := r.updateUserPassExpirationPolicy(ctx, cr, internalSecrets, user); err != nil {
		return err
	}

	log.Info("Password changed, updating user", "user", user.Name)

	err := r.updateProxyUser(cr, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update Proxy users")
	}
	log.Info("Proxy user updated", "user", user.Name)

	orig := internalSecrets.DeepCopy()
	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Patch(context.TODO(), internalSecrets, client.MergeFrom(orig))
	if err != nil {
		return errors.Wrap(err, "update internal users secrets proxyadmin user password")
	}
	log.Info("Internal secrets updated", "user", user.Name)

	actions.restartProxySQL = true

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
