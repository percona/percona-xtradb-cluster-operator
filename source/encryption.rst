Data at rest encryption
************************

`Ful data at rest encryption in Percona XtraDB Cluster <https://www.percona.com/doc/percona-xtradb-cluster/LATEST/management/data_at_rest_encryption.html>`_ is supported by the Operator since version 1.4.0.

..note:: "Data at rest" means inactive data stored as files, database records, etc.

To implement these features, the Operator uses ``keyring_vault`` plugin,
which ships with Percona XtraDB Cluster, and utilizes `HashiCorp Vault <https://www.vaultproject.io/>`_ authentication server. 

.. contents:: :local:

.. _install-vault:

Installing and configuring Vault
-------------------------------

The following steps will deploy Vault on Kubernetes with the `Helm 3 package manager <https://helm.sh/>`_:

1. Clone the official HashiCorp Vault Helm chart from GitHub:

   .. code:: bash

      $ git clone https://github.com/hashicorp/vault-helm.git
      $ cd vault-helm

2. Checkout to a tagged Vault release version to install it with Helm:

   .. code:: bash

      $ git checkout v0.4.0

3. Now use Helm to do the installation:

   .. code:: bash

      $ helm install --name vault-service --namespace vault-namespace ./

4. After the installation, Vauld should be first initialized and then unsealed.
   Initializing Vault is done with the following commands:

   .. code:: bash

      $ kubectl exec -it pod/vault-service-0 -- vault operator init -key-shares=1 -key-threshold=1 -format=json > /tmp/vault-init
      $ unsealKey=$(jq -r ".unseal_keys_b64[]" < /tmp/vault-init)
      $ token=$(jq -r ".root_token" < /tmp/vault-init)

   To unseal Vault, execute the following command **for each Pod** of Vault
   running: 

   .. code:: bash

      $ kubectl exec -it pod/vault-service-0 -- vault operator unseal "$unsealKey"

5. Now it is time to enable secrets within Vault. First, get the Vault root
   token:

   .. code:: bash

      $ cat /tmp/vault-init | jq -r ".root_token"

   The output will be like follows:

   .. code:: text

      s.VgQvaXl8xGFO1RUxAPbPbsfN

   Now login to Vault with this token and enable secrets:

      $ kubectl exec -it vault-0 -- /bin/sh
      $ vault login s.VgQvaXl8xGFO1RUxAPbPbsfN
      $ vault secrets enable --version=1 -path=secret kv

   .. note:: You can also enable audit, which is not mandatory, but useful:

      .. code:: bash

         $ vault audit enable file file_path=/vault/vault-audit.log

6. To enable Vault secret within Kubernetes, create and apply the YAML file as
   follows::

      apiVersion: v1
      kind: Secret
      metadata:
        name: some-name-vault
      type: Opaque
      stringData:
        keyring_vault.conf: |-
          token = s.VgQvaXl8xGFO1RUxAPbPbsfN
          vault_url = vault-service.vault-service.svc.cluster.local
          secret_mount_point = secret

More details on how to install and configure Vault can be found `in the official documentation <https://learn.hashicorp.com/vault?track=getting-started-k8s#getting-started-k8s>`_.

.. _vault-encryption:

Using the encryption
-------------------------------

If using Percona XtraDB Cluster 5.7, you should turn encryption on explicitly
when you create a table or a tablespace. This can be done by adding the
``ENCRYPTION='Y'`` part to your SQL statement, like in the following example:

   .. code:: sql

      CREATE TABLE t1 (c1 INT, PRIMARY KEY pk(c1)) ENCRYPTION='Y';
      CREATE TABLESPACE foo ADD DATAFILE 'foo.ibd' ENCRYPTION='Y';

If using Percona XtraDB Cluster 8.0, the encryption is turned on by default.
