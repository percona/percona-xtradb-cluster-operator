package pxc

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
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

	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Update(context.TODO(), internalSecrets)
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

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	logger.Info(fmt.Sprintf("User %s: password changed, updating user", user.Name))

	err := r.updateUserPassWithoutDP(cr, secrets, internalSecrets, user)
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

func (r *ReconcilePerconaXtraDBCluster) handleMonitorUserWithoutDP(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	logger := r.logger(cr.Name, cr.Namespace)

	user := &users.SysUser{
		Name:  users.Monitor,
		Pass:  string(secrets.Data[users.Monitor]),
		Hosts: []string{"%"},
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
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

	logger.Info(fmt.Sprintf("User %s: password changed, updating user", user.Name))

	err = r.updateUserPassWithoutDP(cr, secrets, internalSecrets, user)
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

func (r *ReconcilePerconaXtraDBCluster) handleClustercheckUserWithoutDP(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	logger := r.logger(cr.Name, cr.Namespace)

	user := &users.SysUser{
		Name:  users.Clustercheck,
		Pass:  string(secrets.Data[users.Clustercheck]),
		Hosts: []string{"localhost"},
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

	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Update(context.TODO(), internalSecrets)
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

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) {
		return nil
	}

	logger.Info(fmt.Sprintf("User %s: password changed, updating user", user.Name))

	err := r.updateUserPassWithoutDP(cr, secrets, internalSecrets, user)
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

func (r *ReconcilePerconaXtraDBCluster) handleReplicationUserWithoutDP(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	logger := r.logger(cr.Name, cr.Namespace)

	if cr.CompareVersionWith("1.9.0") < 0 {
		return nil
	}

	user := &users.SysUser{
		Name:  users.Replication,
		Pass:  string(secrets.Data[users.Replication]),
		Hosts: []string{"%"},
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

	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Update(context.TODO(), internalSecrets)
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
