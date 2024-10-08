package pxc

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

		if user.PasswordSecretRef != nil && user.PasswordSecretRef.Name == "" {
			log.Error(nil, "passwordSecretRef name is not set", "user", user.Name)
			continue
		}

		if user.PasswordSecretRef != nil && user.PasswordSecretRef.Key == "" {
			user.PasswordSecretRef.Key = "password"
		}

		userSecretName := ""
		userSecretPassKey := ""

		if user.PasswordSecretRef == nil {
			userSecretName = fmt.Sprintf("%s-custom-user-secret", cr.Name)
			userSecretPassKey = user.Name + "-pass"

		} else {
			userSecretName = user.PasswordSecretRef.Name
			userSecretPassKey = user.PasswordSecretRef.Key
		}

		userSecret, err := getUserSecret(ctx, r.client, cr, userSecretName)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				userSecret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userSecretName,
						Namespace: cr.Namespace,
					},
				}
				err := generateUserPass(ctx, r.client, cr, userSecret, userSecretPassKey)
				if err != nil {
					return errors.Wrap(err, "failed to generate user password secrets")
				}
			} else {
				log.Error(err, "failed to get user secret", "user", user)
				continue
			}
		}

		annotationKey := fmt.Sprintf("percona.com/%s-%s-hash", cr.Name, user.Name)
		if userSecret.Annotations == nil {
			userSecret.Annotations = make(map[string]string)
		}

		if userPasswordChanged(userSecret, annotationKey, userSecretPassKey) {
			log.Info("User password changed", "user", user.Name)

			err := um.Exec(ctx, alterUserQuery(&user, string(userSecret.Data[userSecretPassKey])))
			if err != nil {
				log.Error(err, "failed to update user", "user", user)
				continue
			}

			userSecret.Annotations[annotationKey] = string(sha256Hash(userSecret.Data[userSecretPassKey]))
			if err := r.client.Update(ctx, userSecret); err != nil {
				return errors.Wrap(err, "update user secret")
			}

			log.Info("User password updated", "user", user.Name)
		}

		us, err := um.GetUsers(ctx, user.Name)
		if err != nil {
			log.Error(err, "failed to get user", "user", user)
			continue
		}

		if userChanged(us, &user) {
			log.Info("Creating/updating user", "user", user.Name)

			err := um.Exec(ctx, upsertUserQuery(&user, string(userSecret.Data[userSecretPassKey])))
			if err != nil {
				log.Error(err, "failed to update user", "user", user)
				continue
			}

			userSecret.Annotations[annotationKey] = string(sha256Hash(userSecret.Data[userSecretPassKey]))
			if err := r.client.Update(ctx, userSecret); err != nil {
				return errors.Wrap(err, "update user secret")
			}

			log.Info("User created/updated", "user", user.Name)
		}
	}

	return nil
}

func generateUserPass(
	ctx context.Context,
	cl client.Client,
	cr *api.PerconaXtraDBCluster,
	secret *corev1.Secret,
	passKey string) error {

	log := logf.FromContext(ctx)

	pass, err := generatePass()
	if err != nil {
		return errors.Wrap(err, "generate custom user password")
	}

	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data[passKey] = pass

	err = cl.Create(ctx, secret)
	if err != nil {
		return fmt.Errorf("create custom users secret: %v", err)
	}

	log.Info("Created custom user secrets", "secrets", cr.Spec.SecretsName)
	return nil
}

func userPasswordChanged(secret *corev1.Secret, key, passKey string) bool {
	if secret.Annotations == nil {
		return false
	}

	hash, ok := secret.Annotations[key]
	if !ok {
		return false
	}

	newHash := sha256Hash(secret.Data[passKey])

	return hash != newHash
}

func userChanged(current []users.User, new *api.User) bool {
	if len(current) == 0 {
		return true
	}

	if len(new.Hosts) == 0 {
		new.Hosts = []string{"%"}
	}

	if len(current) != len(new.Hosts) {
		return true
	}

	newHosts := make(map[string]struct{}, len(new.Hosts))
	for _, h := range new.Hosts {
		newHosts[h] = struct{}{}
	}

	for _, u := range current {
		if _, ok := newHosts[u.Host]; !ok {
			return true
		}
	}

	return false
}

func getUserSecret(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBCluster, name string) (*corev1.Secret, error) {
	secrets := corev1.Secret{}
	err := cl.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, &secrets)
	return &secrets, errors.Wrap(err, "get user secrets")
}

func sysUserNames() map[string]struct{} {
	sysUserNames := make(map[string]struct{}, len(users.UserNames))
	for _, v := range users.UserNames {
		sysUserNames[string(v)] = struct{}{}
	}
	return sysUserNames
}

func alterUserQuery(user *api.User, pass string) []string {
	query := make([]string, 0)

	if len(user.Hosts) > 0 {
		for _, host := range user.Hosts {
			query = append(query, fmt.Sprintf("ALTER USER '%s'@'%s' IDENTIFIED BY '%s'", user.Name, host, pass))
		}
	} else {
		query = append(query, fmt.Sprintf("ALTER USER '%s'@'%%' IDENTIFIED BY '%s'", user.Name, pass))
	}

	return query
}

func upsertUserQuery(user *api.User, pass string) []string {
	query := make([]string, 0)

	for _, db := range user.DBs {
		query = append(query, (fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", db)))
	}

	withGrantOption := ""
	if user.WithGrantOption {
		withGrantOption = "WITH GRANT OPTION"
	}

	if len(user.Hosts) > 0 {
		for _, host := range user.Hosts {
			query = append(query, (fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%s' IDENTIFIED BY '%s'", user.Name, host, pass)))

			if len(user.Grants) > 0 {
				grants := strings.Join(user.Grants, ",")
				if len(user.DBs) > 0 {
					for _, db := range user.DBs {
						query = append(query, (fmt.Sprintf("GRANT %s ON %s.* TO '%s'@'%s' %s", grants, db, user.Name, host, withGrantOption)))
					}
				} else {
					query = append(query, (fmt.Sprintf("GRANT %s ON *.* TO '%s'@'%s' %s", grants, user.Name, host, withGrantOption)))
				}
			}
		}
	} else {
		query = append(query, (fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s'", user.Name, pass)))

		if len(user.Grants) > 0 {
			grants := strings.Join(user.Grants, ",")
			if len(user.DBs) > 0 {
				for _, db := range user.DBs {
					query = append(query, (fmt.Sprintf("GRANT %s ON %s.* TO '%s'@'%%' %s", grants, db, user.Name, withGrantOption)))
				}
			} else {
				query = append(query, (fmt.Sprintf("GRANT %s ON *.* TO '%s'@'%%' %s", grants, user.Name, withGrantOption)))
			}
		}
	}

	return query
}
