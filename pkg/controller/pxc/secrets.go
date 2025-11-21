package pxc

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	mrand "math/rand"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

const internalSecretsPrefix = "internal-"

// reconciles the user secret provided in `.spec.secretName` field, and returns the updated/created secret.
func (r *ReconcilePerconaXtraDBCluster) reconcileUsersSecret(ctx context.Context, cr *api.PerconaXtraDBCluster) (*corev1.Secret, error) {
	log := logf.FromContext(ctx)

	secretObj := new(corev1.Secret)
	err := r.client.Get(ctx,
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.SecretsName,
		},
		secretObj,
	)
	if err == nil {
		if err := validatePasswords(secretObj); err != nil {
			return nil, errors.Wrap(err, "validate passwords")
		}
		isChanged, err := setUserSecretDefaults(secretObj, cr.Spec.PasswordGenerationOptions)
		if err != nil {
			return nil, errors.Wrap(err, "set user secret defaults")
		}
		if isChanged {
			err := r.client.Update(ctx, secretObj)
			if err == nil {
				log.Info("User secrets updated", "secrets", cr.Spec.SecretsName)
			}
			return secretObj, err
		}
		return secretObj, nil
	} else if !k8serror.IsNotFound(err) {
		return nil, errors.Wrap(err, "get secret")
	}

	secretObj = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.SecretsName,
			Namespace: cr.Namespace,
			Labels:    naming.LabelsCluster(cr),
		},
		Type: corev1.SecretTypeOpaque,
	}

	if _, err = setUserSecretDefaults(secretObj, cr.Spec.PasswordGenerationOptions); err != nil {
		return nil, errors.Wrap(err, "set user secret defaults")
	}

	err = r.client.Create(ctx, secretObj)
	if err != nil {
		return nil, fmt.Errorf("create Users secret: %v", err)
	}

	log.Info("Created user secrets", "secrets", cr.Spec.SecretsName)
	return secretObj, nil
}

func setUserSecretDefaults(secret *corev1.Secret, secretsOptions *api.PasswordGenerationOptions) (isChanged bool, err error) {
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	users := []string{users.Root, users.Xtrabackup, users.Monitor, users.ProxyAdmin, users.Operator, users.Replication}
	for _, user := range users {
		if pass, ok := secret.Data[user]; !ok || len(pass) == 0 {
			secret.Data[user], err = generatePass(secretsOptions)
			if err != nil {
				return false, errors.Wrapf(err, "create %s users password", user)
			}

			isChanged = true
		}
	}
	return
}

const (
	passBaseSymbols = "ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789"
)

// generatePass generates a random password with or without special symbols
func generatePass(secretsOptions *api.PasswordGenerationOptions) ([]byte, error) {
	mrand.Seed(time.Now().UnixNano())
	ln := mrand.Intn(secretsOptions.MaxLength-secretsOptions.MinLength) + secretsOptions.MinLength
	b := make([]byte, ln)
	for i := 0; i < ln; i++ {
		passSymbols := passBaseSymbols + secretsOptions.Symbols
		randInt, err := rand.Int(rand.Reader, big.NewInt(int64(len(passSymbols))))
		if err != nil {
			return nil, errors.Wrap(err, "get rand int")
		}
		b[i] = passSymbols[randInt.Int64()]
	}

	return b, nil
}

func validatePasswords(secret *corev1.Secret) error {
	for user, pass := range secret.Data {
		switch user {
		case users.ProxyAdmin:
			if strings.ContainsAny(string(pass), ";:") {
				return errors.New("invalid proxyadmin password, don't use ';' or ':'")
			}
			if strings.HasPrefix(string(pass), "*") {
				return errors.New("invalid proxyadmin password, first character must not be '*'")
			}
		default:
			continue
		}
	}

	return nil
}
