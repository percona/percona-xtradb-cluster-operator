.. _encryption:

Data-at-Rest Encryption
************************

`Full data-at-rest encryption in Percona XtraDB Cluster <https://www.percona.com/doc/percona-xtradb-cluster/LATEST/management/data_at_rest_encryption.html>`_ is supported by the Operator since version 1.4.0.

..note:: `Data at rest <https://en.wikipedia.org/wiki/Data_at_rest>`_ means inactive data stored as files, database records, etc.

To implement these features, the Operator uses ``keyring_vault`` plugin,
which ships with Percona XtraDB Cluster, and utilizes `HashiCorp Vault <https://www.vaultproject.io/>`_ storage for encryption keys.

.. contents:: :local:

.. _install-vault:

Installing Vault
----------------

The following steps will deploy Vault on Kubernetes with the `Helm 3 package manager <https://helm.sh/>`_. Other Vault installation methods should also work, so the instruction placed here is not obligatory and is for illustration purposes.

1. Clone the official HashiCorp Vault Helm chart from GitHub:

   .. code:: bash

      $ git clone -b v0.4.0 https://github.com/hashicorp/vault-helm.git
      $ cd vault-helm

2. Now use Helm to do the installation:

   .. code:: bash

      $ helm install --name vault-service ./

3. After the installation, Vauld should be first initialized and then unsealed.
   Initializing Vault is done with the following commands:

   .. code:: bash

      $ kubectl exec -it pod/vault-service-0 -- vault operator init -key-shares=1 -key-threshold=1 -format=json > /tmp/vault-init
      $ unsealKey=$(jq -r ".unseal_keys_b64[]" < /tmp/vault-init)

   To unseal Vault, execute the following command **for each Pod** of Vault
   running:

   .. code:: bash

      $ kubectl exec -it pod/vault-service-0 -- vault operator unseal "$unsealKey"

.. _configure-vault:

Configuring Vault
-----------------

1. First, you should enable secrets within Vault. Get the Vault root token:

   .. code:: bash

      $ cat /tmp/vault-init | jq -r ".root_token"

   The output will be like follows:

   .. code:: text

      s.VgQvaXl8xGFO1RUxAPbPbsfN

   Now login to Vault with this token and enable the "pxc-secret" secrets path:

      $ kubectl exec -it vault-0 -- /bin/sh
      $ vault login s.VgQvaXl8xGFO1RUxAPbPbsfN
      $ vault secrets enable --version=1 -path=pxc-secret kv

   .. note:: You can also enable audit, which is not mandatory, but useful:

      .. code:: bash

         $ vault audit enable file file_path=/vault/vault-audit.log

2. To enable Vault secret within Kubernetes, create and apply the YAML file,
   as described further.

   2.1. To access the Vault server via HTTP, follow the next YAML file example:

      .. code:: text

         apiVersion: v1
         kind: Secret
         metadata:
           name: some-name-vault
         type: Opaque
         stringData:
           keyring_vault.conf: |-
              token = s.VgQvaXl8xGFO1RUxAPbPbsfN
              vault_url = vault-service.vault-service.svc.cluster.local
              secret_mount_point = pxc-secret

      .. note:: the ``name`` key in the above file should be equal to the
         ``spec.vaultSecretName`` key from the ``deploy/cr.yaml`` configuration
         file.

   2.2. To turn on TLS and access the Vault server via HTTPS, you should do two more things:
      
      * add one more item to the secret: the contents of the ``ca.cert`` file
        with your certificate,
      * store the path to this file in the ``vault_ca`` key.

      .. code:: text

         apiVersion: v1
         kind: Secret
         metadata:
           name: some-name-vault
         type: Opaque
         stringData:
           keyring_vault.conf: |-
             token = = s.VgQvaXl8xGFO1RUxAPbPbsfN
             vault_url = https://vault-service.vault-service.svc.cluster.local
             secret_mount_point = pxc-secret
             vault_ca = /etc/mysql/vault-keyring-secret/ca.cert
           ca.cert: |-
             -----BEGIN CERTIFICATE-----
             MIIEczCCA1ugAwIBAgIBADANBgkqhkiG9w0BAQQFAD..AkGA1UEBhMCR0Ix
             EzARBgNVBAgTClNvbWUtU3RhdGUxFDASBgNVBAoTC0..0EgTHRkMTcwNQYD
             VQQLEy5DbGFzcyAxIFB1YmxpYyBQcmltYXJ5IENlcn..XRpb24gQXV0aG9y
             aXR5MRQwEgYDVQQDEwtCZXN0IENBIEx0ZDAeFw0wMD..TUwMTZaFw0wMTAy
             MDQxOTUwMTZaMIGHMQswCQYDVQQGEwJHQjETMBEGA1..29tZS1TdGF0ZTEU
             MBIGA1UEChMLQmVzdCBDQSBMdGQxNzA1BgNVBAsTLk..DEgUHVibGljIFBy
             aW1hcnkgQ2VydGlmaWNhdGlvbiBBdXRob3JpdHkxFD..AMTC0Jlc3QgQ0Eg
             THRkMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCg..Tz2mr7SZiAMfQyu
             vBjM9OiJjRazXBZ1BjP5CE/Wm/Rr500PRK+Lh9x5eJ../ANBE0sTK0ZsDGM
             ak2m1g7oruI3dY3VHqIxFTz0Ta1d+NAjwnLe4nOb7/..k05ShhBrJGBKKxb
             8n104o/5p8HAsZPdzbFMIyNjJzBM2o5y5A13wiLitE..fyYkQzaxCw0Awzl
             kVHiIyCuaF4wj571pSzkv6sv+4IDMbT/XpCo8L6wTa..sh+etLD6FtTjYbb
             rvZ8RQM1tlKdoMHg2qxraAV++HNBYmNWs0duEdjUbJ..XI9TtnS4o1Ckj7P
             OfljiQIDAQABo4HnMIHkMB0GA1UdDgQWBBQ8urMCRL..5AkIp9NJHJw5TCB
             tAYDVR0jBIGsMIGpgBQ8urMCRLYYMHUKU5AkIp9NJH..aSBijCBhzELMAkG
             A1UEBhMCR0IxEzARBgNVBAgTClNvbWUtU3RhdGUxFD..AoTC0Jlc3QgQ0Eg
             THRkMTcwNQYDVQQLEy5DbGFzcyAxIFB1YmxpYyBQcm..ENlcnRpZmljYXRp
             b24gQXV0aG9yaXR5MRQwEgYDVQQDEwtCZXN0IENBIE..DAMBgNVHRMEBTAD
             AQH/MA0GCSqGSIb3DQEBBAUAA4IBAQC1uYBcsSncwA..DCsQer772C2ucpX
             xQUE/C0pWWm6gDkwd5D0DSMDJRqV/weoZ4wC6B73f5..bLhGYHaXJeSD6Kr
             XcoOwLdSaGmJYslLKZB3ZIDEp0wYTGhgteb6JFiTtn..sf2xdrYfPCiIB7g
             BMAV7Gzdc4VspS6ljrAhbiiawdBiQlQmsBeFz9JkF4..b3l8BoGN+qMa56Y
             It8una2gY4l2O//on88r5IWJlm1L0oA8e4fR2yrBHX..adsGeFKkyNrwGi/
             7vQMfXdGsRrXNGRGnX+vWDZ3/zWI0joDtCkNnqEpVn..HoX
             -----END CERTIFICATE-----

      .. note:: the ``name`` key in the above file should be equal to the
         ``spec.vaultSecretName`` key from the ``deploy/cr.yaml`` configuration
         file.
         
      .. note:: For techincal reasons the ``vault_ca`` key should either exist
         or not exist in the YAML file; commented option like
         ``#vault_ca = ...`` is not acceptable.

More details on how to install and configure Vault can be found `in the official documentation <https://learn.hashicorp.com/vault?track=getting-started-k8s#getting-started-k8s>`_.

.. _vault-encryption:

Using the encryption
--------------------

If using Percona XtraDB Cluster 5.7, you should turn encryption on explicitly
when you create a table or a tablespace. This can be done by adding the
``ENCRYPTION='Y'`` part to your SQL statement, like in the following example:

   .. code:: sql

      CREATE TABLE t1 (c1 INT, PRIMARY KEY pk(c1)) ENCRYPTION='Y';
      CREATE TABLESPACE foo ADD DATAFILE 'foo.ibd' ENCRYPTION='Y';

.. note:: See more details on encryption in Percona XtraDB Cluster 5.7 `here <https://www.percona.com/doc/percona-xtradb-cluster/5.7/management/data_at_rest_encryption.html>`_.

If using Percona XtraDB Cluster 8.0, the encryption is turned on by default.

