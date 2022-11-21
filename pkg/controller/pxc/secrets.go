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

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

const internalSecretsPrefix = "internal-"

func (r *ReconcilePerconaXtraDBCluster) reconcileUsersSecret(cr *api.PerconaXtraDBCluster) error {
	logger := r.logger(cr.Name, cr.Namespace)

	secretObj := new(corev1.Secret)
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.SecretsName,
		},
		secretObj,
	)
	if err == nil {
		if err := validatePasswords(secretObj); err != nil {
			return errors.Wrap(err, "validate passwords")
		}
		isChanged, err := setUserSecretDefaults(secretObj)
		if err != nil {
			return errors.Wrap(err, "set user secret defaults")
		}
		if isChanged {
			err := r.client.Update(context.TODO(), secretObj)
			if err == nil {
				logger.Info(fmt.Sprintf("User secrets updated: %s", cr.Spec.SecretsName))
			}
			return err
		}
		return nil
	} else if !k8serror.IsNotFound(err) {
		return errors.Wrap(err, "get secret")
	}

	secretObj = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.SecretsName,
			Namespace: cr.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
	}

	if _, err = setUserSecretDefaults(secretObj); err != nil {
		return errors.Wrap(err, "set user secret defaults")
	}

	err = r.client.Create(context.TODO(), secretObj)
	if err != nil {
		return fmt.Errorf("create Users secret: %v", err)
	}

	logger.Info(fmt.Sprintf("Created user secrets: %s", cr.Spec.SecretsName))
	return nil
}

func setUserSecretDefaults(secret *corev1.Secret) (isChanged bool, err error) {
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	users := []string{"root", "xtrabackup", "monitor", "clustercheck", "proxyadmin", "operator", "replication"}
	for _, user := range users {
		if pass, ok := secret.Data[user]; !ok || len(pass) == 0 {
			secret.Data[user], err = generatePass()
			if err != nil {
				return false, errors.Wrapf(err, "create %s users password", user)
			}

			isChanged = true
		}
	}
	return
}

const (
	passwordMaxLen = 20
	passwordMinLen = 16
	passSymbols    = "ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789" +
		"!#$%&()*+,-.<=>?@[]^_{}~"
)

// generatePass generates a random password
func generatePass() ([]byte, error) {
	mrand.Seed(time.Now().UnixNano())
	ln := mrand.Intn(passwordMaxLen-passwordMinLen) + passwordMinLen
	b := make([]byte, ln)
	for i := 0; i < ln; i++ {
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
		default:
			continue
		}
	}

	return nil
}
