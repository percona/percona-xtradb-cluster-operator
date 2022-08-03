package pxc

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"

	cm "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxctls"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ReconcilePerconaXtraDBCluster) reconcileSSL(cr *api.PerconaXtraDBCluster) error {
	if cr.Spec.AllowUnsafeConfig && (cr.Spec.TLS == nil || cr.Spec.TLS.IssuerConf == nil) {
		return nil
	}
	secretObj := corev1.Secret{}
	secretInternalObj := corev1.Secret{}
	errSecret := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.PXC.SSLSecretName,
		},
		&secretObj,
	)
	errInternalSecret := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.PXC.SSLInternalSecretName,
		},
		&secretInternalObj,
	)
	if errSecret == nil && errInternalSecret == nil {
		return nil
	} else if errSecret != nil && !k8serr.IsNotFound(errSecret) {
		return fmt.Errorf("get secret: %v", errSecret)
	} else if errInternalSecret != nil && !k8serr.IsNotFound(errInternalSecret) {
		return fmt.Errorf("get internal secret: %v", errInternalSecret)
	}
	// don't create secret ssl-internal if secret ssl is not created by operator
	if errSecret == nil && !metav1.IsControlledBy(&secretObj, cr) {
		return nil
	}
	err := r.createSSLByCertManager(cr)
	if err != nil {
		if cr.Spec.TLS != nil && cr.Spec.TLS.IssuerConf != nil {
			return fmt.Errorf("create ssl with cert manager %w", err)
		}
		err = r.createSSLManualy(cr)
		if err != nil {
			return fmt.Errorf("create ssl internally: %v", err)
		}
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) createSSLByCertManager(cr *api.PerconaXtraDBCluster) error {
	issuerName := cr.Name + "-pxc-issuer"
	caIssuerName := cr.Name + "-pxc-ca-issuer"
	issuerKind := "Issuer"
	issuerGroup := ""
	if cr.Spec.TLS != nil && cr.Spec.TLS.IssuerConf != nil {
		issuerKind = cr.Spec.TLS.IssuerConf.Kind
		issuerName = cr.Spec.TLS.IssuerConf.Name
		issuerGroup = cr.Spec.TLS.IssuerConf.Group
	} else {
		if err := r.createIssuer(cr.Namespace, caIssuerName, ""); err != nil {
			return err
		}

		caCert := &cm.Certificate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cr.Name + "-ca-cert",
				Namespace: cr.Namespace,
			},
			Spec: cm.CertificateSpec{
				SecretName: cr.Name + "-ca-cert",
				CommonName: cr.Name + "-ca",
				IsCA:       true,
				IssuerRef: cmmeta.ObjectReference{
					Name:  caIssuerName,
					Kind:  issuerKind,
					Group: issuerGroup,
				},
				Duration:    &metav1.Duration{Duration: time.Hour * 24 * 365},
				RenewBefore: &metav1.Duration{Duration: 730 * time.Hour},
			},
		}

		err := r.client.Create(context.TODO(), caCert)
		if err != nil && !k8serr.IsAlreadyExists(err) {
			return fmt.Errorf("create CA certificate: %v", err)
		}

		if err := r.waitForCerts(cr.Namespace, caCert.Spec.SecretName); err != nil {
			return err
		}

		if err := r.createIssuer(cr.Namespace, issuerName, caCert.Spec.SecretName); err != nil {
			return err
		}
	}

	kubeCert := &cm.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-ssl",
			Namespace: cr.Namespace,
		},
		Spec: cm.CertificateSpec{
			SecretName: cr.Spec.PXC.SSLSecretName,
			CommonName: cr.Name + "-proxysql",
			DNSNames: []string{
				cr.Name + "-pxc",
				cr.Name + "-proxysql",
				"*." + cr.Name + "-pxc",
				"*." + cr.Name + "-proxysql",
			},
			IsCA: true,
			IssuerRef: cmmeta.ObjectReference{
				Name:  issuerName,
				Kind:  issuerKind,
				Group: issuerGroup,
			},
		},
	}

	if cr.Spec.TLS != nil && len(cr.Spec.TLS.SANs) > 0 {
		kubeCert.Spec.DNSNames = append(kubeCert.Spec.DNSNames, cr.Spec.TLS.SANs...)
	}

	err := r.client.Create(context.TODO(), kubeCert)
	if err != nil && !k8serr.IsAlreadyExists(err) {
		return fmt.Errorf("create certificate: %v", err)
	}

	if cr.Spec.PXC.SSLSecretName == cr.Spec.PXC.SSLInternalSecretName {
		return r.waitForCerts(cr.Namespace, cr.Spec.PXC.SSLSecretName)
	}

	kubeCert = &cm.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-ssl-internal",
			Namespace: cr.Namespace,
		},
		Spec: cm.CertificateSpec{
			SecretName: cr.Spec.PXC.SSLInternalSecretName,
			CommonName: cr.Name + "-pxc",
			DNSNames: []string{
				cr.Name + "-pxc",
				"*." + cr.Name + "-pxc",
				cr.Name + "-haproxy-replicas." + cr.Namespace + ".svc.cluster.local",
				cr.Name + "-haproxy-replicas." + cr.Namespace,
				cr.Name + "-haproxy-replicas",
				cr.Name + "-haproxy." + cr.Namespace + ".svc.cluster.local",
				cr.Name + "-haproxy." + cr.Namespace,
				cr.Name + "-haproxy",
			},
			IsCA: true,
			IssuerRef: cmmeta.ObjectReference{
				Name:  issuerName,
				Kind:  issuerKind,
				Group: issuerGroup,
			},
		},
	}
	if cr.Spec.TLS != nil && len(cr.Spec.TLS.SANs) > 0 {
		kubeCert.Spec.DNSNames = append(kubeCert.Spec.DNSNames, cr.Spec.TLS.SANs...)
	}
	err = r.client.Create(context.TODO(), kubeCert)
	if err != nil && !k8serr.IsAlreadyExists(err) {
		return fmt.Errorf("create internal certificate: %v", err)
	}

	return r.waitForCerts(cr.Namespace, cr.Spec.PXC.SSLSecretName, cr.Spec.PXC.SSLInternalSecretName)
}

func (r *ReconcilePerconaXtraDBCluster) waitForCerts(namespace string, secretsList ...string) error {
	ticker := time.NewTicker(3 * time.Second)
	timeoutTimer := time.NewTimer(30 * time.Second)
	defer timeoutTimer.Stop()
	defer ticker.Stop()
	for {
		select {
		case <-timeoutTimer.C:
			return errors.Errorf("timeout: can't get tls certificates from certmanager, %s", secretsList)
		case <-ticker.C:
			sucessCount := 0
			for _, secretName := range secretsList {
				secret := &corev1.Secret{}
				err := r.client.Get(context.TODO(), types.NamespacedName{
					Name:      secretName,
					Namespace: namespace,
				}, secret)
				if err != nil && !k8serr.IsNotFound(err) {
					return err
				} else if err == nil {
					sucessCount++
				}
			}
			if sucessCount == len(secretsList) {
				return nil
			}
		}
	}
}

func (r *ReconcilePerconaXtraDBCluster) createIssuer(namespace, issuer string, caCertSecret string) error {
	spec := cm.IssuerSpec{}

	if caCertSecret == "" {
		spec = cm.IssuerSpec{
			IssuerConfig: cm.IssuerConfig{
				SelfSigned: &cm.SelfSignedIssuer{},
			},
		}
	} else {
		spec = cm.IssuerSpec{
			IssuerConfig: cm.IssuerConfig{
				CA: &cm.CAIssuer{SecretName: caCertSecret},
			},
		}
	}

	err := r.client.Create(context.TODO(), &cm.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      issuer,
			Namespace: namespace,
		},
		Spec: spec,
	})
	if err != nil && !k8serr.IsAlreadyExists(err) {
		return fmt.Errorf("create issuer: %v", err)
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
	if cr.Spec.TLS != nil && len(cr.Spec.TLS.SANs) > 0 {
		proxyHosts = append(proxyHosts, cr.Spec.TLS.SANs...)
	}
	caCert, tlsCert, key, err := pxctls.Issue(proxyHosts)
	if err != nil {
		return fmt.Errorf("create proxy certificate: %v", err)
	}
	data["ca.crt"] = caCert
	data["tls.crt"] = tlsCert
	data["tls.key"] = key
	owner, err := OwnerRef(cr, r.scheme)
	if err != nil {
		return err
	}
	ownerReferences := []metav1.OwnerReference{owner}
	secretObj := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            cr.Spec.PXC.SSLSecretName,
			Namespace:       cr.Namespace,
			OwnerReferences: ownerReferences,
		},
		Data: data,
		Type: corev1.SecretTypeTLS,
	}
	err = r.client.Create(context.TODO(), &secretObj)
	if err != nil && !k8serr.IsAlreadyExists(err) {
		return fmt.Errorf("create TLS secret: %v", err)
	}
	pxcHosts := []string{
		cr.Name + "-pxc",
		"*." + cr.Name + "-pxc",
		cr.Name + "-haproxy-replicas." + cr.Namespace + ".svc.cluster.local",
		cr.Name + "-haproxy-replicas." + cr.Namespace,
		cr.Name + "-haproxy-replicas",
		cr.Name + "-haproxy." + cr.Namespace + ".svc.cluster.local",
		cr.Name + "-haproxy." + cr.Namespace,
		cr.Name + "-haproxy",
	}
	if cr.Spec.TLS != nil && len(cr.Spec.TLS.SANs) > 0 {
		pxcHosts = append(pxcHosts, cr.Spec.TLS.SANs...)
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
			OwnerReferences: ownerReferences,
		},
		Data: data,
		Type: corev1.SecretTypeTLS,
	}
	err = r.client.Create(context.TODO(), &secretObjInternal)
	if err != nil && !k8serr.IsAlreadyExists(err) {
		return fmt.Errorf("create TLS internal secret: %v", err)
	}
	return nil
}
