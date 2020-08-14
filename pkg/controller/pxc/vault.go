package pxc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"strings"
	"time"

	dlog "log"

	api "github.com/hashicorp/vault/api"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *ReconcilePerconaXtraDBCluster) IssueVaultToken(rootVaultSercet corev1.Secret) error {
	data := string(rootVaultSercet.Data["keyring_vault.conf"])
	fields := strings.Split(data, "\n")
	conf := make(map[string]string)
	for _, f := range fields {
		kv := strings.Split(f, "=")
		conf[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
	}
	dlog.Println(conf)

	newData := make(map[string][]byte)

	tr := &http.Transport{}
	if ca, ok := rootVaultSercet.Data["ca.cert"]; ok {
		newData["ca.cert"] = ca

		certPool, err := x509.SystemCertPool()
		if err != nil {
			return fmt.Errorf("failed to get system cert pool: %v", err)
		}

		ok := certPool.AppendCertsFromPEM(ca)
		if !ok {
			return fmt.Errorf("failed to append cert")
		}

		tr.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
			RootCAs:            certPool,
		}
	}

	var httpClient = &http.Client{
		Timeout:   10 * time.Second,
		Transport: tr,
	}
	cli, err := api.NewClient(&api.Config{
		HttpClient: httpClient,
		Address:    conf["vault_url"],
	})
	if err != nil {
		return fmt.Errorf("failed to create vault client: %v", err)
	}

	cli.SetToken(conf["token"])
	policy := fmt.Sprintf(`
path "%s/%s"
{
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "%s/%s/*"
{
  capabilities = ["create", "read", "update", "delete", "list"]
}
`, conf["secret_mount_point"], rootVaultSercet.Namespace,
		conf["secret_mount_point"], rootVaultSercet.Namespace)

	cli.Sys().PutPolicy(rootVaultSercet.Namespace, policy)
	sec, err := cli.Auth().Token().Create(&api.TokenCreateRequest{
		Policies: []string{rootVaultSercet.Namespace},
	})
	if err != nil {
		return fmt.Errorf("failed to create token: %v", err)
	}

	dlog.Printf("%#v", sec.Auth)
	token := sec.Auth.ClientToken
	newData["keyring_vault.conf"] = []byte(
		fmt.Sprintf(`
token = %s
vault_url = %s
secret_mount_point = %s
vault_ca = %s`,
			token,
			conf["vault_url"],
			conf["secret_mount_point"]+"/"+rootVaultSercet.Namespace,
			conf["vault_ca"],
		))

	secretObj := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rootVaultSercet.Name + "-new",
			Namespace: rootVaultSercet.Namespace, // pass as a parameter
		},
		Data: newData,
		Type: corev1.SecretTypeOpaque,
	}

	err = r.client.Create(context.TODO(), &secretObj)
	if err != nil {
		return fmt.Errorf("create token secret: %v", err)
	}

	return nil
}
