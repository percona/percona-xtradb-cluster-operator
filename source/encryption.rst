.. _encryption:

Data-at-Rest Encryption
************************

`Full data-at-rest encryption in Percona XtraDB Cluster <https://www.percona.com/doc/percona-xtradb-cluster/LATEST/management/data_at_rest_encryption.html>`_ is supported by the Operator since version 1.4.0.

.. note:: `Data at rest <https://en.wikipedia.org/wiki/Data_at_rest>`_ means inactive data stored as files, database records, etc.

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

      $ helm install vault-service ./

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

   .. code:: bash

      $ kubectl exec -it vault-service-0 -- /bin/sh
      $ vault login s.VgQvaXl8xGFO1RUxAPbPbsfN
      $ vault secrets enable --version=1 -path=pxc-secret kv

   .. note:: You can also enable audit, which is not mandatory, but useful:

      .. code:: bash

         $ vault audit enable file file_path=/vault/vault-audit.log

2. To enable Vault secret within Kubernetes, create and apply the YAML file,
   as described further.

   2.1. To access the Vault server via HTTP, follow the next YAML file example:

      .. code:: yaml

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

      .. code:: yaml

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

If using *Percona XtraDB Cluster* 5.7, you should turn encryption on explicitly
when you create a table or a tablespace. This can be done by adding the
``ENCRYPTION='Y'`` part to your SQL statement, like in the following example:

   .. code:: sql

      CREATE TABLE t1 (c1 INT, PRIMARY KEY pk(c1)) ENCRYPTION='Y';
      CREATE TABLESPACE foo ADD DATAFILE 'foo.ibd' ENCRYPTION='Y';

.. note:: See more details on encryption in Percona XtraDB Cluster 5.7 `here <https://www.percona.com/doc/percona-xtradb-cluster/5.7/management/data_at_rest_encryption.html>`_.

If using *Percona XtraDB Cluster* 8.0, the encryption is turned on by default.
The following table presents the default values of the `correspondent my.cnf
configuration options <https://www.percona.com/doc/percona-server/LATEST/security/data-at-rest-encryption.html>`_:

+---------------------------------------------------+-------------------------------------------------------+
| Option                                            | Default value                                         |
+===================================================+=======================================================+
| ``early-plugin-load``                             | ``keyring_vault.so``                                  |
+---------------------------------------------------+-------------------------------------------------------+
| ``keyring_vault_config``                          | ``/etc/mysql/vault-keyring-secret/keyring_vault.conf``|
+---------------------------------------------------+-------------------------------------------------------+
| ``default_table_encryption``                      | ``ON``                                                |
+---------------------------------------------------+-------------------------------------------------------+
| ``table_encryption_privilege_check``              | ``ON``                                                |
+---------------------------------------------------+-------------------------------------------------------+
| ``innodb_undo_log_encrypt``                       | ``ON``                                                |
+---------------------------------------------------+-------------------------------------------------------+
| ``innodb_redo_log_encrypt``                       | ``ON``                                                |
+---------------------------------------------------+-------------------------------------------------------+
| ``binlog_encryption``                             | ``ON``                                                |
+---------------------------------------------------+-------------------------------------------------------+
| ``binlog_rotate_encryption_master_key_at_startup``| ``ON``                                                |
+---------------------------------------------------+-------------------------------------------------------+
| ``innodb_temp_tablespace_encrypt``                | ``ON``                                                |
+---------------------------------------------------+-------------------------------------------------------+
| ``innodb_parallel_dblwr_encrypt``                 | ``ON``                                                |
+---------------------------------------------------+-------------------------------------------------------+
| ``innodb_encrypt_online_alter_logs``              | ``ON``                                                |
+---------------------------------------------------+-------------------------------------------------------+
| ``encrypt_tmp_files``                             | ``ON``                                                |
+---------------------------------------------------+-------------------------------------------------------+
