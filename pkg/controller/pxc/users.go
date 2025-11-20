package pxc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

var mysql80 = version.Must(version.NewVersion("8.0.0"))

// https://dev.mysql.com/doc/refman/8.0/en/privileges-provided.html#priv_system-user
var privSystemUserAddedIn = version.Must(version.NewVersion("8.0.16"))

var PassNotPropagatedError = errors.New("password not yet propagated")

type userUpdateActions struct {
	restartPXC            bool
	restartProxySQL       bool
	restartHAProxy        bool
	updateReplicationPass bool
}

type ReconcileUsersResult struct {
	pxcAnnotations            map[string]string
	proxysqlAnnotations       map[string]string
	haproxyAnnotations        map[string]string
	updateReplicationPassword bool
}

func (r *ReconcilePerconaXtraDBCluster) reconcileUsers(
	ctx context.Context,
	cr *api.PerconaXtraDBCluster,
	secrets *corev1.Secret,
) (*ReconcileUsersResult, error) {
	log := logf.FromContext(ctx)

	internalSecretName := internalSecretsPrefix + cr.Name
	internalSecrets := corev1.Secret{}
	err := r.client.Get(ctx,
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

	mysqlVersion := cr.Status.PXC.Version
	if mysqlVersion == "" {
		var err error
		mysqlVersion, err = r.mysqlVersion(ctx, cr, statefulset.NewNode(cr))
		if err != nil {
			if errors.Is(err, versionNotReadyErr) {
				return nil, nil
			}
			return nil, errors.Wrap(err, "retrieving pxc version")
		}
	}

	ver, err := version.NewVersion(mysqlVersion)
	if err != nil {
		return nil, errors.Wrap(err, "invalid pxc version")
	}

	var actions *userUpdateActions
	if ver.GreaterThanOrEqual(mysql80) {
		actions, err = r.updateUsers(ctx, cr, secrets, &internalSecrets)
		if err != nil {
			return nil, errors.Wrap(err, "manage sys users")
		}
	} else {
		actions, err = r.updateUsersWithoutDP(ctx, cr, secrets, &internalSecrets)
		if err != nil {
			return nil, errors.Wrap(err, "manage sys users")
		}
	}

	newSysData, err := json.Marshal(secrets.Data)
	if err != nil {
		return nil, errors.Wrap(err, "marshal sys secret data")
	}
	newSecretDataHash := sha256Hash(newSysData)

	result := &ReconcileUsersResult{
		updateReplicationPassword: actions.updateReplicationPass,
	}

	if actions.restartProxySQL && cr.ProxySQLEnabled() {
		log.Info("Proxy pods will be restarted", "last-applied-secret", newSecretDataHash)
		result.proxysqlAnnotations = map[string]string{"last-applied-secret": newSecretDataHash}
	}
	if actions.restartPXC {
		log.Info("PXC pods will be restarted", "last-applied-secret", newSecretDataHash)
		result.pxcAnnotations = map[string]string{"last-applied-secret": newSecretDataHash}
	}
	if actions.restartHAProxy && cr.HAProxyEnabled() {
		log.Info("HAProxy pods will be restarted", "last-applied-secret", newSecretDataHash)
		result.haproxyAnnotations = map[string]string{"last-applied-secret": newSecretDataHash}
	}

	return result, nil
}

func sha256Hash(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func (r *ReconcilePerconaXtraDBCluster) updateUsers(ctx context.Context, cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret) (*userUpdateActions, error) {
	res := &userUpdateActions{}

	for _, u := range users.UserNames {
		if _, ok := secrets.Data[u]; !ok {
			continue
		}

		switch u {
		case users.Root:
			if err := r.handleRootUser(ctx, cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Operator:
			if err := r.handleOperatorUser(ctx, cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Monitor:
			if err := r.handleMonitorUser(ctx, cr, secrets, internalSecrets, res); err != nil {
				if errors.Is(err, PassNotPropagatedError) {
					continue
				}
				return res, err
			}
		case users.Xtrabackup:
			if err := r.handleXtrabackupUser(ctx, cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.Replication:
			if err := r.handleReplicationUser(ctx, cr, secrets, internalSecrets, res); err != nil {
				return res, err
			}
		case users.ProxyAdmin:
			if err := r.handleProxyadminUser(ctx, cr, secrets, internalSecrets, res); err != nil {
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

func (r *ReconcilePerconaXtraDBCluster) handleRootUser(ctx context.Context, cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	log := logf.FromContext(ctx)

	user := &users.SysUser{
		Name:  users.Root,
		Pass:  string(secrets.Data[users.Root]),
		Hosts: []string{"localhost", "%"},
	}

	if cr.Status.Status != api.AppStateReady && !r.invalidPasswordApplied(cr.Status) {
		return nil
	}

	if err := r.updateUserPassExpirationPolicy(ctx, cr, internalSecrets, user); err != nil {
		return err
	}

	passDiscarded, err := r.isOldPasswordDiscarded(cr, internalSecrets, user)
	if err != nil {
		return err
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) && passDiscarded {
		return nil
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) && !passDiscarded {
		err = r.discardOldPassword(cr, secrets, internalSecrets, user)
		if err != nil {
			return errors.Wrap(err, "discard old pass")
		}
		log.Info("Old password discarded", "user", user.Name)

		return nil
	}

	log.Info("Password changed, updating user", "user", user.Name)

	err = r.updateUserPassWithRetention(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update root users pass")
	}
	log.Info("Password updated", "user", user.Name)

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

	err = r.discardOldPassword(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "discard old password")
	}
	log.Info("Old password discarded", "user", user.Name)

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleOperatorUser(ctx context.Context, cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
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

	passDiscarded, err := r.isOldPasswordDiscarded(cr, internalSecrets, user)
	if err != nil {
		return err
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) && passDiscarded {
		return nil
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) && !passDiscarded {
		err = r.discardOldPassword(cr, secrets, internalSecrets, user)
		if err != nil {
			return errors.Wrap(err, "discard old pass")
		}
		log.Info("Old password discarded", "user", user.Name)

		return nil
	}

	log.Info("Password changed, updating user", "user", user.Name)

	err = r.updateUserPassWithRetention(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update operator users pass")
	}
	log.Info("Password updated", "user", user.Name)

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

	err = r.discardOldPassword(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "discard operator old password")
	}
	log.Info("Old password discarded", "user", user.Name)

	return nil
}

// manageOperatorAdminUser ensures that operator user is always present and with the right privileges
func (r *ReconcilePerconaXtraDBCluster) manageOperatorAdminUser(ctx context.Context, cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret) error {
	log := logf.FromContext(ctx)

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

	pass, err := generatePass(cr.Spec.PasswordGenerationOptions)
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

	log.Info("User created and privileges granted", "user", users.Operator)
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleMonitorUser(ctx context.Context, cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
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

	passDiscarded, err := r.isOldPasswordDiscarded(cr, internalSecrets, user)
	if err != nil {
		return err
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) && passDiscarded {
		return nil
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) && !passDiscarded {
		log.Info("Password updated but old one not discarded", "user", user.Name)

		passPropagated, err := r.isPassPropagated(cr, user)
		if err != nil {
			return errors.Wrap(err, "is password propagated")
		}
		if !passPropagated {
			return PassNotPropagatedError
		}

		actions.restartProxySQL = true
		if cr.Spec.PMM != nil && cr.Spec.PMM.IsEnabled(internalSecrets) {
			actions.restartPXC = true
		}
		if cr.Spec.PXC.Sidecars != nil && cr.Spec.PXC.HasSidecarInternalSecret(internalSecrets) {
			actions.restartPXC = true
		}

		err = r.discardOldPassword(cr, secrets, internalSecrets, user)
		if err != nil {
			return errors.Wrap(err, "discard old pass")
		}
		log.Info("Old password discarded", "user", user.Name)

		return nil
	}

	log.Info("Password changed, updating user", "user", user.Name)

	err = r.updateUserPassWithRetention(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update monitor users pass")
	}
	log.Info("Password updated", "user", user.Name)

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

	actions.restartProxySQL = true
	if cr.Spec.PMM != nil && cr.Spec.PMM.IsEnabled(internalSecrets) {
		actions.restartPXC = true
	}

	orig := internalSecrets.DeepCopy()
	internalSecrets.Data[user.Name] = secrets.Data[user.Name]
	err = r.client.Patch(context.TODO(), internalSecrets, client.MergeFrom(orig))
	if err != nil {
		return errors.Wrap(err, "update internal users secrets monitor user password")
	}
	log.Info("Internal secrets updated", "user", user.Name)

	passPropagated, err := r.isPassPropagated(cr, user)
	if err != nil {
		return errors.Wrap(err, "is password propagated")
	}
	if !passPropagated {
		return PassNotPropagatedError
	}

	err = r.discardOldPassword(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "discard monitor old password")
	}
	log.Info("Old password discarded", "user", user.Name)

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) updateMonitorUserGrant(ctx context.Context, cr *api.PerconaXtraDBCluster, internalSysSecretObj *corev1.Secret, um *users.Manager) error {
	log := logf.FromContext(ctx)

	annotationName := "grant-for-1.6.0-monitor-user"
	if internalSysSecretObj.Annotations[annotationName] == "done" {
		return nil
	}

	err := um.Update160MonitorUserGrant(string(internalSysSecretObj.Data[users.Monitor]))
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

	log.Info("User monitor: granted privileges")
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleXtrabackupUser(ctx context.Context, cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
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

	passDiscarded, err := r.isOldPasswordDiscarded(cr, internalSecrets, user)
	if err != nil {
		return err
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) && passDiscarded {
		return nil
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) && !passDiscarded {
		err = r.discardOldPassword(cr, secrets, internalSecrets, user)
		if err != nil {
			return errors.Wrap(err, "discard old pass")
		}
		log.Info("Old password discarded", "user", user.Name)

		return nil
	}

	log.Info("Password changed, updating user", "user", user.Name)

	err = r.updateUserPassWithRetention(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update xtrabackup users pass")
	}
	log.Info("Password updated", "user", user.Name)

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

	err = r.discardOldPassword(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "discard xtrabackup old pass")
	}
	log.Info("Old password discarded", "user", user.Name)

	actions.restartPXC = true
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) updateXtrabackupUserGrant(ctx context.Context, cr *api.PerconaXtraDBCluster, secrets *corev1.Secret) error {
	log := logf.FromContext(ctx)

	annotationName := "grant-for-1.15.0-xtrabackup-user"
	if secrets.Annotations[annotationName] == "done" {
		return nil
	}

	um, err := getUserManager(cr, secrets)
	if err != nil {
		return err
	}
	defer um.Close()

	err = um.Update1150XtrabackupUser(string(secrets.Data[users.Xtrabackup]))
	if err != nil {
		return errors.Wrap(err, "update xtrabackup grant")
	}

	if secrets.Annotations == nil {
		secrets.Annotations = make(map[string]string)
	}

	secrets.Annotations[annotationName] = "done"
	err = r.client.Update(context.TODO(), secrets)
	if err != nil {
		return errors.Wrap(err, "update internal sys users secret annotation")
	}

	log.Info("User xtrabackup: granted privileges")
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleReplicationUser(ctx context.Context, cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	log := logf.FromContext(ctx)

	if cr.CompareVersionWith("1.9.0") < 0 {
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

	passDiscarded, err := r.isOldPasswordDiscarded(cr, internalSecrets, user)
	if err != nil {
		return err
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) && passDiscarded {
		return nil
	}

	if bytes.Equal(secrets.Data[user.Name], internalSecrets.Data[user.Name]) && !passDiscarded {
		err = r.discardOldPassword(cr, secrets, internalSecrets, user)
		if err != nil {
			return errors.Wrap(err, "discard old pass")
		}
		log.Info("Old password discarded", "user", user.Name)

		return nil
	}

	log.Info("Password changed, updating user", "user", user.Name)

	err = r.updateUserPassWithRetention(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "update replication users pass")
	}
	log.Info("Password updated", "user", user.Name)

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

	err = r.discardOldPassword(cr, secrets, internalSecrets, user)
	if err != nil {
		return errors.Wrap(err, "discard replicaiton old pass")
	}
	log.Info("Old password discarded", "user", user.Name)

	actions.updateReplicationPass = true
	return nil
}

// manageReplicationUser ensures that replication user is always present and with the right privileges
func (r *ReconcilePerconaXtraDBCluster) manageReplicationUser(ctx context.Context, cr *api.PerconaXtraDBCluster, sysUsersSecretObj, secrets *corev1.Secret) error {
	log := logf.FromContext(ctx)

	pass, existInSys := sysUsersSecretObj.Data[users.Replication]
	_, existInInternal := secrets.Data[users.Replication]
	if existInSys && !existInInternal {
		if secrets.Data == nil {
			secrets.Data = make(map[string][]byte)
		}
		secrets.Data[users.Replication] = pass
		return nil
	}
	if existInSys {
		return nil
	}

	um, err := getUserManager(cr, secrets)
	if err != nil {
		return err
	}
	defer um.Close()

	pass, err = generatePass(cr.Spec.PasswordGenerationOptions)
	if err != nil {
		return errors.Wrap(err, "generate password")
	}

	err = um.CreateReplicationUser(string(pass))
	if err != nil {
		return errors.Wrap(err, "create replication user")
	}

	sysUsersSecretObj.Data[users.Replication] = pass
	secrets.Data[users.Replication] = pass

	err = r.client.Update(context.TODO(), sysUsersSecretObj)
	if err != nil {
		return errors.Wrap(err, "update sys users secret")
	}
	err = r.client.Update(context.TODO(), secrets)
	if err != nil {
		return errors.Wrap(err, "update internal users secret")
	}

	log.Info("User replication: user created and privileges granted")
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleProxyadminUser(ctx context.Context, cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
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

func (r *ReconcilePerconaXtraDBCluster) handlePMMUser(ctx context.Context, cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	log := logf.FromContext(ctx)

	if cr.Spec.PMM == nil || !cr.Spec.PMM.IsEnabled(secrets) {
		return nil
	}

	if key, ok := secrets.Data[users.PMMServerKey]; ok {
		if _, ok := internalSecrets.Data[users.PMMServerKey]; !ok {
			internalSecrets.Data[users.PMMServerKey] = key

			err := r.client.Update(context.TODO(), internalSecrets)
			if err != nil {
				return errors.Wrap(err, "update internal users secrets pmm user password")
			}
			log.Info("Internal secrets updated", "user", users.PMMServerKey)

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

	if cr.Status.Status != api.AppStateReady && !r.invalidPasswordApplied(cr.Status) {
		return nil
	}

	log.Info("Password changed, updating user", "user", name)

	orig := internalSecrets.DeepCopy()
	internalSecrets.Data[name] = secrets.Data[name]
	err := r.client.Patch(context.TODO(), internalSecrets, client.MergeFrom(orig))
	if err != nil {
		return errors.Wrap(err, "update internal users secrets pmm user password")
	}
	log.Info("Internal secrets updated", "user", name)

	actions.restartPXC = true
	actions.restartProxySQL = true
	actions.restartHAProxy = true

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handlePMM3User(ctx context.Context, cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, actions *userUpdateActions) error {
	log := logf.FromContext(ctx)

	if cr.Spec.PMM == nil || !cr.Spec.PMM.Enabled {
		return nil
	}

	if key, ok := secrets.Data[users.PMMServerToken]; ok {
		if _, ok := internalSecrets.Data[users.PMMServerToken]; !ok {
			internalSecrets.Data[users.PMMServerToken] = key

			err := r.client.Update(ctx, internalSecrets)
			if err != nil {
				return errors.Wrap(err, "update internal users secrets pmm user token")
			}
			log.Info("Internal secrets updated", "user", users.PMMServerToken)

			return nil
		}
	}

	name := users.PMMServerToken

	if bytes.Equal(secrets.Data[name], internalSecrets.Data[name]) {
		return nil
	}

	if cr.Status.Status != api.AppStateReady && !r.invalidPasswordApplied(cr.Status) {
		return nil
	}

	log.Info("Password changed, updating user", "user", name)

	orig := internalSecrets.DeepCopy()
	internalSecrets.Data[name] = secrets.Data[name]
	err := r.client.Patch(ctx, internalSecrets, client.MergeFrom(orig))
	if err != nil {
		return errors.Wrap(err, "update internal users secrets pmm user token")
	}
	log.Info("Internal secrets updated", "user", name)

	actions.restartPXC = true
	actions.restartProxySQL = true
	actions.restartHAProxy = true

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) syncPXCUsersWithProxySQL(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
	log := logf.FromContext(ctx)

	if !cr.Spec.ProxySQLEnabled() || cr.Status.PXC.Ready < 1 {
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

	log.V(1).Info("PXC users synced with ProxySQL")
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) updateUserPassWithRetention(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, user *users.SysUser) error {
	um, err := getUserManager(cr, internalSecrets)
	if err != nil {
		return err
	}
	defer um.Close()

	err = um.UpdateUserPass(user)
	if err != nil {
		return errors.Wrap(err, "update user pass")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) discardOldPassword(cr *api.PerconaXtraDBCluster, secrets, internalSecrets *corev1.Secret, user *users.SysUser) error {
	um, err := getUserManager(cr, internalSecrets)
	if err != nil {
		return err
	}
	defer um.Close()

	err = um.DiscardOldPassword(user)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("discard old user %s pass", user.Name))
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) isOldPasswordDiscarded(cr *api.PerconaXtraDBCluster, secrets *corev1.Secret, user *users.SysUser) (bool, error) {
	um, err := getUserManager(cr, secrets)
	if err != nil {
		return false, err
	}
	defer um.Close()

	discarded, err := um.IsOldPassDiscarded(user)
	if err != nil {
		return false, errors.Wrap(err, "is old password discarded")
	}

	return discarded, nil
}

func (r *ReconcilePerconaXtraDBCluster) isPassPropagated(cr *api.PerconaXtraDBCluster, user *users.SysUser) (bool, error) {
	components := map[string]int32{
		"pxc": cr.Spec.PXC.Size,
	}

	if cr.HAProxyEnabled() {
		components["haproxy"] = cr.Spec.HAProxy.Size
	}

	eg := new(errgroup.Group)

	for component, size := range components {
		comp := component
		compCount := size
		eg.Go(func() error {
			for i := 0; int32(i) < compCount; i++ {
				pod := corev1.Pod{}
				err := r.client.Get(context.TODO(),
					types.NamespacedName{
						Namespace: cr.Namespace,
						Name:      fmt.Sprintf("%s-%s-%d", cr.Name, comp, i),
					},
					&pod,
				)
				if err != nil && k8serrors.IsNotFound(err) {
					return err
				} else if err != nil {
					return errors.Wrapf(err, "get %s pod", comp)
				}
				var errb, outb bytes.Buffer
				err = r.clientcmd.Exec(&pod, comp, []string{"cat", fmt.Sprintf("/etc/mysql/mysql-users-secret/%s", user.Name)}, nil, &outb, &errb, false)
				if err != nil {
					return errors.Errorf("exec cat on %s-%d: %v / %s / %s", comp, i, err, outb.String(), errb.String())
				}
				if len(errb.Bytes()) > 0 {
					return errors.Errorf("cat on %s-%d: %s", comp, i, errb.String())
				}

				if outb.String() != user.Pass {
					return PassNotPropagatedError
				}
			}

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		if err == PassNotPropagatedError {
			return false, nil
		}
		return false, err
	}

	return true, nil
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

func (r *ReconcilePerconaXtraDBCluster) grantMonitorUserPrivilege(ctx context.Context, cr *api.PerconaXtraDBCluster, internalSysSecretObj *corev1.Secret, um *users.Manager) error {
	log := logf.FromContext(ctx)

	annotationName := "grant-for-1.10.0-system-privilege"
	if internalSysSecretObj.Annotations[annotationName] == "done" {
		return nil
	}

	if err := um.Update1100MonitorUserPrivilege(); err != nil {
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

	log.Info("monitor user privileges granted")
	return nil
}

func getUserManager(cr *api.PerconaXtraDBCluster, secrets *corev1.Secret) (*users.Manager, error) {
	pxcUser := users.Root
	pxcPass := string(secrets.Data[users.Root])
	if _, ok := secrets.Data[users.Operator]; ok {
		pxcUser = users.Operator
		pxcPass = string(secrets.Data[users.Operator])
	}

	addr := cr.Name + "-pxc-unready." + cr.Namespace + ":3306"
	hasKey, err := cr.ConfigHasKey("mysqld", "proxy_protocol_networks")
	if err != nil {
		return nil, errors.Wrap(err, "check if congfig has proxy_protocol_networks key")
	}
	if hasKey {
		addr = cr.Name + "-pxc-unready." + cr.Namespace + ":33062"
	}

	um, err := users.NewManager(addr, pxcUser, pxcPass, cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
	if err != nil {
		return nil, errors.Wrap(err, "new users manager")
	}

	return &um, nil
}

func (r *ReconcilePerconaXtraDBCluster) updateUserPassExpirationPolicy(ctx context.Context, cr *api.PerconaXtraDBCluster, internalSecrets *corev1.Secret, user *users.SysUser) error {
	log := logf.FromContext(ctx)

	annotationName := "pass-expire-policy-for-1.13.0-user-" + user.Name
	if internalSecrets.Annotations[annotationName] == "done" {
		return nil
	}

	if cr.CompareVersionWith("1.13.0") >= 0 {
		um, err := getUserManager(cr, internalSecrets)
		if err != nil {
			return err
		}

		if err := um.UpdatePassExpirationPolicy(user); err != nil {
			return errors.Wrapf(err, "update %s user password expiration policy", user.Name)
		}

		if internalSecrets.Annotations == nil {
			internalSecrets.Annotations = make(map[string]string)
		}

		internalSecrets.Annotations[annotationName] = "done"
		err = r.client.Update(ctx, internalSecrets)
		if err != nil {
			return errors.Wrap(err, "update internal sys users secret annotation")
		}

		log.Info("Password expiration policy updated", "user", user.Name)
		return nil
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) invalidPasswordApplied(status api.PerconaXtraDBClusterStatus) bool {
	if len(status.Messages) == 0 {
		return false
	}

	if strings.Contains(status.Messages[0], "password does not satisfy the current policy") {
		return true
	}

	return false
}

func (r *ReconcilePerconaXtraDBCluster) updateMySQLInitFile(ctx context.Context, cr *api.PerconaXtraDBCluster, internalSecret *corev1.Secret, user *users.SysUser) error {
	log := logf.FromContext(ctx)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-mysql-init",
			Namespace: cr.Namespace,
		},
	}
	data := map[string][]byte{
		"init.sql": []byte("SET SESSION wsrep_on=OFF;\nSET SESSION sql_log_bin=0;\n"),
	}
	if err := r.client.Get(ctx, client.ObjectKeyFromObject(secret), secret); err == nil {
		data = secret.Data
	}

	statements := make([]string, 0)
	for _, host := range user.Hosts {
		statements = append(statements, fmt.Sprintf("ALTER USER '%s'@'%s' IDENTIFIED BY '%s';\n", user.Name, host, user.Pass))
	}

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.client, secret, func() error {
		data["init.sql"] = append(data["init.sql"], []byte(strings.Join(statements, ""))...)
		secret.Data = data
		return nil
	})

	log.Info(fmt.Sprintf("MySQL init secret %s", opResult), "secret", secret.Name, "user", user.Name)

	return err
}
