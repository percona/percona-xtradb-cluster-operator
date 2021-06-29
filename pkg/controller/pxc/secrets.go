package pxc

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	mrand "math/rand"
	"time"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ReconcilePerconaXtraDBCluster) reconcileUsersSecret(cr *api.PerconaXtraDBCluster) error {
	secretObj := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.SecretsName,
		},
		&secretObj,
	)
	if err == nil {
		return nil
	} else if !k8serror.IsNotFound(err) {
		return errors.Wrap(err, "get secret")
	}

	data := make(map[string][]byte)
	data["root"], err = generatePass()
	if err != nil {
		return errors.Wrap(err, "create root users password")
	}
	data["xtrabackup"], err = generatePass()
	if err != nil {
		return errors.Wrap(err, "create xtrabackup users password")
	}
	data["monitor"], err = generatePass()
	if err != nil {
		return errors.Wrap(err, "create monitor users password")
	}
	data["clustercheck"], err = generatePass()
	if err != nil {
		return errors.Wrap(err, "create clustercheck users password")
	}
	data["proxyadmin"], err = generatePass()
	if err != nil {
		return errors.Wrap(err, "create proxyadmin users password")
	}
	data["operator"], err = generatePass()
	if err != nil {
		return errors.Wrap(err, "create operator users password")
	}
	data["replication"], err = generatePass()
	if err != nil {
		return errors.Wrap(err, "generate replication password")
	}

	secretObj = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.SecretsName,
			Namespace: cr.Namespace,
		},
		Data: data,
		Type: corev1.SecretTypeOpaque,
	}
	err = r.client.Create(context.TODO(), &secretObj)
	if err != nil {
		return fmt.Errorf("create Users secret: %v", err)
	}
	return nil
}

const (
	passwordMaxLen = 20
	passwordMinLen = 16
	passSymbols    = "ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789"
)

//generatePass generate random password
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
