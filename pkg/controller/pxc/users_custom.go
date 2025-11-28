package pxc

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
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
	err := r.client.Get(ctx,
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
		if user.Name == "" {
			log.Error(nil, "user name is not set", "user", user)
			continue
		}

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

		if len(user.Hosts) == 0 {
			user.Hosts = []string{"%"}
		}

		defaultUserSecretName := fmt.Sprintf("%s-custom-user-secret", cr.Name)

		userSecretName := defaultUserSecretName
		userSecretPassKey := user.Name
		if user.PasswordSecretRef != nil {
			userSecretName = user.PasswordSecretRef.Name
			userSecretPassKey = user.PasswordSecretRef.Key
		}

		userSecret, err := getUserSecret(ctx, r.client, cr, userSecretName, defaultUserSecretName, userSecretPassKey)
		if err != nil {
			log.Error(err, "failed to get user secret", "user", user)
			continue
		}

		annotationKey := fmt.Sprintf("percona.com/%s-%s-hash", cr.Name, user.Name)

		u, err := um.GetUser(ctx, user.Name)
		if err != nil {
			log.Error(err, "failed to get user", "user", user)
			continue
		}

		if userPasswordChanged(userSecret, u, annotationKey, userSecretPassKey) {
			log.Info("User password changed", "user", user.Name)

			err := um.UpsertUser(ctx, alterUserQuery(&user), string(userSecret.Data[userSecretPassKey]))
			if err != nil {
				log.Error(err, "failed to update user", "user", user)
				continue
			}

			err = k8s.AnnotateObject(ctx, r.client, userSecret,
				map[string]string{annotationKey: sha256Hash(userSecret.Data[userSecretPassKey])})
			if err != nil {
				return errors.Wrap(err, "update user secret")
			}

			log.Info("User password updated", "user", user.Name)
		}

		if userChanged(u, &user, log) {
			log.Info("Creating/updating user", "user", user.Name)

			err := um.UpsertUser(ctx, upsertUserQuery(&user), string(userSecret.Data[userSecretPassKey]))
			if err != nil {
				log.Error(err, "failed to update user", "user", user)
				continue
			}

			err = k8s.AnnotateObject(ctx, r.client, userSecret,
				map[string]string{annotationKey: sha256Hash(userSecret.Data[userSecretPassKey])})
			if err != nil {
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
	name string,
	passKey string,
) error {
	log := logf.FromContext(ctx)

	pass, err := generatePass(name, cr.Spec.PasswordGenerationOptions)
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

func userPasswordChanged(secret *corev1.Secret, dbUser *users.User, key, passKey string) bool {
	if secret.Annotations == nil {
		// If annotations are nil and the user is created (not nil),
		// we assume that password has changed.
		return dbUser != nil
	}

	hash, ok := secret.Annotations[key]
	if !ok {
		// If annotation is not present in the secret and the user is created (not nil),
		// we assume that password has changed.
		return dbUser != nil
	}

	newHash := sha256Hash(secret.Data[passKey])
	return hash != newHash
}

func userChanged(current *users.User, desired *api.User, log logr.Logger) bool {
	userName := desired.Name

	if current == nil {
		log.Info("User not created", "user", userName)
		return true
	}

	for _, u := range desired.Hosts {
		if !current.Hosts.Has(u) {
			log.Info("Hosts changed", "current", current.Hosts, "desired", desired.Hosts, "user", userName)
			return true
		}
	}

	for _, db := range desired.DBs {
		if !current.DBs.Has(db) {
			log.Info("DBs changed", "current", current.DBs, "desired", desired.DBs, "user", userName)
			return true
		}
	}

	for _, host := range desired.Hosts {
		if _, ok := current.Grants[host]; !ok && len(desired.Grants) > 0 {
			log.Info("Grants for user host not present", "host", host, "user", userName)
			return true
		}

		for _, grant := range desired.Grants {
			for _, currGrant := range current.Grants[host] {
				if currGrant == fmt.Sprintf("GRANT USAGE ON *.* TO `%s`@`%s`", desired.Name, host) {
					continue
				}

				if !strings.Contains(currGrant, strings.ToUpper(grant)) {
					log.Info("Grant not present in current grants", "grant", grant, "user", userName)
					return true
				}

				if desired.WithGrantOption && !strings.Contains(currGrant, "WITH GRANT OPTION") {
					log.Info("Grant with grant option not present", "user", userName)
					return true
				}
			}
		}

		for _, db := range desired.DBs {
			dbPresent := false

			for _, currGrant := range current.Grants[host] {
				if strings.Contains(currGrant, fmt.Sprintf("ON `%s`.*", db)) {
					dbPresent = true
					break
				}
			}

			if !dbPresent {
				log.Info("DB not present in current grants", "db", db, "user", userName)
				return true
			}
		}
	}

	return false
}

// getUserSecret gets secret by name defined by `user.PasswordSecretRef.Name` or returns a secret
// with newly generated password if name matches defaultName
func getUserSecret(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBCluster, name, defaultName, passKey string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := cl.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, secret)

	if err != nil && name != defaultName {
		return nil, errors.Wrap(err, "failed to get user secret")
	}

	if err != nil && !k8serrors.IsNotFound(err) && name == defaultName {
		return nil, errors.Wrap(err, "failed to get default user secret")
	}

	if err != nil && k8serrors.IsNotFound(err) {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: cr.Namespace,
			},
		}
		err := generateUserPass(ctx, cl, cr, secret, name, passKey)
		if err != nil {
			return nil, errors.Wrap(err, "failed to generate user password secrets")
		}

		return secret, nil
	}

	_, hasPass := secret.Data[passKey]
	if !hasPass && name == defaultName {
		pass, err := generatePass(name, cr.Spec.PasswordGenerationOptions)
		if err != nil {
			return nil, errors.Wrap(err, "generate custom user password")
		}

		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}

		secret.Data[passKey] = pass

		err = cl.Update(ctx, secret)
		if err != nil {
			return nil, errors.Wrap(err, "failed to update user secret")
		}

		return secret, nil
	}

	// pass key should be present in the user provided secret
	if !hasPass {
		return nil, errors.New("password key not found in secret")
	}

	return secret, nil
}

func sysUserNames() map[string]struct{} {
	sysUserNames := make(map[string]struct{}, len(users.UserNames))
	for _, v := range users.UserNames {
		sysUserNames[string(v)] = struct{}{}
	}
	return sysUserNames
}

func escapeIdentifier(identifier string) string {
	return strings.ReplaceAll(identifier, "'", "''")
}

func alterUserQuery(user *api.User) []string {
	query := make([]string, 0)

	if len(user.Hosts) > 0 {
		for _, host := range user.Hosts {
			query = append(query, fmt.Sprintf("ALTER USER '%s'@'%s' IDENTIFIED BY ?", escapeIdentifier(user.Name), escapeIdentifier(host)))
		}
	} else {
		query = append(query, fmt.Sprintf("ALTER USER '%s'@'%%' IDENTIFIED BY ?", escapeIdentifier(user.Name)))
	}

	return query
}

func upsertUserQuery(user *api.User) []string {
	query := make([]string, 0)

	for _, db := range user.DBs {
		query = append(query, (fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", db)))
	}

	withGrantOption := ""
	if user.WithGrantOption {
		withGrantOption = "WITH GRANT OPTION"
	}

	for _, host := range user.Hosts {
		query = append(query, (fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%s' IDENTIFIED BY ?", escapeIdentifier(user.Name), escapeIdentifier(host))))

		if len(user.Grants) > 0 {
			grants := strings.Join(user.Grants, ",")
			if len(user.DBs) > 0 {
				for _, db := range user.DBs {
					q := fmt.Sprintf("GRANT %s ON %s.* TO '%s'@'%s' %s", grants, db, escapeIdentifier(user.Name), escapeIdentifier(host), withGrantOption)
					query = append(query, q)
				}
			} else {
				q := fmt.Sprintf("GRANT %s ON *.* TO '%s'@'%s' %s", grants, escapeIdentifier(user.Name), escapeIdentifier(host), withGrantOption)
				query = append(query, q)
			}
		}
	}

	return query
}
