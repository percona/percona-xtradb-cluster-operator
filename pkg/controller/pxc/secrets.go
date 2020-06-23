package pxc

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	mrand "math/rand"
	"time"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("get secret: %v", err)
	}

	data := make(map[string][]byte)
	data["root"], err = generatePass()
	if err != nil {
		return fmt.Errorf("create root users password: %v", err)
	}
	data["xtrabackup"], err = generatePass()
	if err != nil {
		return fmt.Errorf("create xtrabackup users password: %v", err)
	}
	data["monitor"], err = generatePass()
	if err != nil {
		return fmt.Errorf("create monitor users password: %v", err)
	}
	data["clustercheck"], err = generatePass()
	if err != nil {
		return fmt.Errorf("create clustercheck users password: %v", err)
	}
	data["proxyadmin"], err = generatePass()
	if err != nil {
		return fmt.Errorf("create proxyadmin users password: %v", err)
	}
	data["operator"], err = generatePass()
	if err != nil {
		return fmt.Errorf("create operator users password: %v", err)
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
			return nil, err
		}
		b[i] = passSymbols[randInt.Int64()]
	}

	return b, nil
}
