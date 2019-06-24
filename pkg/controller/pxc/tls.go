package pxc

import (
	"context"
	"fmt"

	cm "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha1"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxctls"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ReconcilePerconaXtraDBCluster) reconsileSSL(cr *api.PerconaXtraDBCluster) error {
	if cr.Spec.PXC.AllowUnsafeConfig {
		return nil
	}
	secretObj := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.PXC.SSLSecretName,
		},
		&secretObj,
	)
	if err == nil {
		return nil
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("get secret: %v", err)
	}

	err = r.createSSLByCertManager(cr)
	if err != nil {
		log.Info("using cert-manger: " + err.Error())
		err = r.createSSLManualy(cr)
		if err != nil {
			return fmt.Errorf("create ssl internally: %v", err)
		}
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) createSSLByCertManager(cr *api.PerconaXtraDBCluster) error {
	issuerKind := "Issuer"
	issuerName := cr.Name + "-pxc-ca"

	err := r.client.Create(context.TODO(), &cm.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:            issuerName,
			Namespace:       cr.Namespace,
			OwnerReferences: cr.OwnerReferences,
		},
		Spec: cm.IssuerSpec{
			IssuerConfig: cm.IssuerConfig{
				SelfSigned: &cm.SelfSignedIssuer{},
			},
		},
	})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("create issuer: %v", err)
	}

	err = r.client.Create(context.TODO(), &cm.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:            cr.Name + "-ssl",
			Namespace:       cr.Namespace,
			OwnerReferences: cr.OwnerReferences,
		},
		Spec: cm.CertificateSpec{
			SecretName: cr.Spec.PXC.SSLSecretName,
			CommonName: cr.Name + "-proxysql",
			DNSNames: []string{
				cr.Name + "-pxc",
				"*." + cr.Name + "-pxc",
				"*." + cr.Name + "-proxysql",
			},
			IsCA: true,
			IssuerRef: cm.ObjectReference{
				Name: issuerName,
				Kind: issuerKind,
			},
		},
	})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("create certificate: %v", err)
	}

	if cr.Spec.PXC.SSLSecretName == cr.Spec.PXC.SSLInternalSecretName {
		return nil
	}

	err = r.client.Create(context.TODO(), &cm.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:            cr.Name + "-ssl-internal",
			Namespace:       cr.Namespace,
			OwnerReferences: cr.OwnerReferences,
		},
		Spec: cm.CertificateSpec{
			SecretName: cr.Spec.PXC.SSLInternalSecretName,
			CommonName: cr.Name + "-pxc",
			DNSNames: []string{
				"*." + cr.Name + "-pxc",
			},
			IsCA: true,
			IssuerRef: cm.ObjectReference{
				Name: issuerName,
				Kind: issuerKind,
			},
		},
	})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("create internal certificate: %v", err)
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) createSSLManualy(cr *api.PerconaXtraDBCluster) error {
	data := make(map[string][]byte)
	proxyHosts := []string{
		cr.Name + "-pxc",
		cr.Name + "-proxysql",
		"*." + cr.Name + "-pxc",
		"*." + cr.Name + "-proxysql",
	}
	caCert, tlsCert, key, err := pxctls.Issue(proxyHosts)
	if err != nil {
		return fmt.Errorf("create proxy certificate: %v", err)
	}
	data["ca.crt"] = caCert
	data["tls.crt"] = tlsCert
	data["tls.key"] = key
	secretObj := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            cr.Spec.PXC.SSLSecretName,
			Namespace:       cr.Namespace,
			OwnerReferences: cr.OwnerReferences,
		},
		Data: data,
		Type: corev1.SecretTypeTLS,
	}
	err = r.client.Create(context.TODO(), &secretObj)
	if err != nil {
		return fmt.Errorf("create TLS secret: %v", err)
	}
	pxcHosts := []string{
		"*." + cr.Name + "-pxc",
		cr.Name + "-pxc",
	}
	caCert, tlsCert, key, err = pxctls.Issue(pxcHosts)
	if err != nil {
		return fmt.Errorf("create pxc certificate: %v", err)
	}
	data["ca.crt"] = caCert
	data["tls.crt"] = tlsCert
	data["tls.key"] = key
	secretObjInternal := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            cr.Spec.PXC.SSLInternalSecretName,
			Namespace:       cr.Namespace,
			OwnerReferences: cr.OwnerReferences,
		},
		Data: data,
		Type: corev1.SecretTypeTLS,
	}
	err = r.client.Create(context.TODO(), &secretObjInternal)
	if err != nil {
		return fmt.Errorf("create TLS internal secret: %v", err)
	}
	return nil
}
