package pxc

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

func (r *ReconcilePerconaXtraDBCluster) reconcileCustomUsers(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
	if cr.Spec.Users == nil && len(cr.Spec.Users) == 0 {
		return nil
	}

	if cr.Status.Status != api.AppStateReady {
		return nil
	}

	log := logf.FromContext(ctx)

	internalSecrets := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      internalSecretsPrefix + cr.Name,
		},
		&internalSecrets,
	)
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Wrap(err, "get internal sys users secret")
	}

	um, err := getUserManager(cr, &internalSecrets)
	if err != nil {
		return err
	}
	defer um.Close()

	sysUserNames := sysUserNames()

	for _, user := range cr.Spec.Users {
		if _, ok := sysUserNames[user.Name]; ok {
			log.Error(nil, "creating user with reserved user name is forbidden", "user", user.Name)
			continue
		}

		if len(user.Grants) == 0 && user.WithGrantOption {
			log.Error(nil, "withGrantOption is set but no grants are provided", "user", user.Name)
			continue
		}

		if user.PasswordSecretRef.Key == "" {
			user.PasswordSecretRef.Key = "password"
		}

		userSecret := corev1.Secret{}
		if user.PasswordSecretRef == nil {
			// generate pass and create a secret
			// userSecret, err = generateUserPass(ctx, r.client, cr)

		} else {
			userSecret, err = getUserSecret(ctx, r.client, cr, user.PasswordSecretRef.Name)
			if err != nil {
				log.Error(err, "failed to get user secret", "user", user)
				continue
			}
		}

		us, err := um.GetUsers(user.Name)
		if err != nil {
			// log.Error(err, "failed to get user", "user", user)
			continue
		}

		if userChanged(us, &user) {
			err := um.Exec(upsertUserQuery(&user, string(userSecret.Data[user.PasswordSecretRef.Key])))
			if err != nil {
				log.Error(err, "failed to update user", "user", user)
				continue
			}
		}
		// if us is different from user, update/create user

		// Check if password is updated
		// do alter user@host identified by newPass

	}

	return nil
}

func userChanged(current []users.User, new *api.User) bool {
	if len(current) != len(new.Hosts) {
		return false
	}

	newHosts := make(map[string]struct{}, len(new.Hosts))
	for _, h := range new.Hosts {
		newHosts[h] = struct{}{}
	}

	for _, u := range current {
		if _, ok := newHosts[u.Host]; !ok {
			return false
		}
	}

	return true
}

func userGrantsChanged(current []users.User, new *api.User) bool {
	return true
}

func getUserSecret(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBCluster, name string) (corev1.Secret, error) {
	secrets := corev1.Secret{}
	err := cl.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, &secrets)
	return secrets, errors.Wrap(err, "get user secrets")
}

func sysUserNames() map[string]struct{} {
	sysUserNames := make(map[string]struct{}, len(users.UserNames))
	for _, v := range users.UserNames {
		sysUserNames[string(v)] = struct{}{}
	}
	return sysUserNames
}

func upsertUserQuery(user *api.User, pass string) string {
	query := strings.Builder{}

	for _, db := range user.DBs {
		query.WriteString(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s;", db))
	}

	withGrantOption := ""
	if user.WithGrantOption {
		withGrantOption = "WITH GRANT OPTION"
	}

	if len(user.Hosts) > 0 {
		for _, host := range user.Hosts {
			query.WriteString(fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%s' IDENTIFIED BY '%s';", user.Name, host, pass))

			if len(user.Grants) > 0 {
				grants := strings.Join(user.Grants, ",")
				if len(user.DBs) > 0 {
					for _, db := range user.DBs {
						query.WriteString(fmt.Sprintf("GRANT %s ON %s.* TO '%s'@'%s' %s;", grants, db, user.Name, host, withGrantOption))
					}
				} else {
					query.WriteString(fmt.Sprintf("GRANT %s ON *.* TO '%s'@'%s' %s;", grants, user.Name, host, withGrantOption))
				}
			}
		}
	} else {
		query.WriteString(fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s';", user.Name, pass))

		if len(user.Grants) > 0 {
			grants := strings.Join(user.Grants, ",")
			if len(user.DBs) > 0 {
				for _, db := range user.DBs {
					query.WriteString(fmt.Sprintf("GRANT %s ON %s.* TO '%s'@'%%' %s;", grants, db, user.Name, withGrantOption))
				}
			} else {
				query.WriteString(fmt.Sprintf("GRANT %s ON *.* TO '%s'@'%%' %s;", grants, user.Name, withGrantOption))
			}
		}
	}

	return query.String()
}
