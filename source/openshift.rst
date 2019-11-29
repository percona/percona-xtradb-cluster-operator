Install Percona XtraDB Cluster on OpenShift
===========================================

0. First of all, clone the percona-xtradb-cluster-operator repository:

   .. code:: bash

      git clone -b release-{{release}} https://github.com/percona/percona-xtradb-cluster-operator
      cd percona-xtradb-cluster-operator

   .. note:: It is crucial to specify the right branch with the\ `-b`
      option while cloning the code on this step. Please be careful.

1. Now Custom Resource Definition for PXC should be created from the
   ``deploy/crd.yaml`` file. Custom Resource Definition extends the
   standard set of resources which Kubernetes “knows” about with the new
   items (in our case ones which are the core of the operator).

   This step should be done only once; it does not need to be repeated
   with the next Operator deployments, etc.

   .. code:: bash

      $ oc apply -f deploy/crd.yaml

   .. note:: Setting Custom Resource Definition requires your user to
      have cluster-admin role privileges.

   If you want to manage your PXC cluster with a non-privileged user, necessary
   permissions can be granted by applying the next clusterrole:

   .. code:: bash

      $ oc create clusterrole pxc-admin --verb="*" --resource=perconaxtradbclusters.pxc.percona.com,perconaxtradbclusters.pxc.percona.com/status,perconaxtradbclusterbackups.pxc.percona.com,perconaxtradbclusterbackups.pxc.percona.com/status,perconaxtradbclusterrestores.pxc.percona.com,perconaxtradbclusterrestores.pxc.percona.com/status
      $ oc adm policy add-cluster-role-to-user pxc-admin <some-user>

   If you have a `cert-manager <https://docs.cert-manager.io/en/release-0.8/getting-started/install/openshift.html>`_ installed, then you have to execute two more commands to be able to manage your PXC cluster with a non-privileged user:

   .. code:: bash

      $ oc create clusterrole cert-admin --verb="*" --resource=issuers.certmanager.k8s.io,certificates.certmanager.k8s.io
      $ oc adm policy add-cluster-role-to-user cert-admin <some-user>

2. The next thing to do is to create a new ``pxc`` project:

   .. code:: bash

      $ oc new-project pxc

3. Now RBAC (role-based access control) for PXC should be set up from
   the ``deploy/rbac.yaml`` file. Briefly speaking, role-based access is
   based on specifically defined roles and actions corresponding to
   them, allowed to be done on specific Kubernetes resources (details
   about users and roles can be found in `OpenShift
   documentation <https://docs.openshift.com/enterprise/3.0/architecture/additional_concepts/authorization.html>`__).

   .. code:: bash

      $ oc apply -f deploy/rbac.yaml

   Finally, it’s time to start the operator within OpenShift:

   .. code:: bash

      $ oc apply -f deploy/operator.yaml

4. Now that’s time to add the PXC Users secrets to OpenShift. They
   should be placed in the data section of the ``deploy/secrets.yaml``
   file as logins and base64-encoded passwords for the user accounts
   (see `Kubernetes
   documentation <https://kubernetes.io/docs/concepts/configuration/secret/>`__
   for details).

   .. note:: The following command can be used to get base64-encoded
      password from a plain text string:
      ``$ echo -n 'plain-text-password' | base64``

   After editing is finished, users secrets should be created (or
   updated with the new passwords) using the following command:

   .. code:: bash

      $ oc apply -f deploy/secrets.yaml

   More details about secrets can be found in `Users <users.html>`_.

5. Install `cert-manager <https://docs.cert-manager.io/en/release-0.8/getting-started/install/openshift.html>`_ if it is not up and running yet then generate and apply certificates as secrets according to `TLS document <TLS.html>`:

   Pre-generated certificates are awailable in the ``deploy/ssl-secrets.yaml`` secrets file for test purposes, but we strongly recommend avoiding their usage on any production system.
   .. code:: bash

      $ oc apply -f <secrets file>

6. After the operator is started and user secrets are added, Percona
   XtraDB Cluster can be created at any time with the following command:

   .. code:: bash

      $ oc apply -f deploy/cr.yaml

   Creation process will take some time. The process is over when both
   operator and replica set pod have reached their Running status:

   .. code:: bash

      $ oc get pods
      NAME                                              READY   STATUS    RESTARTS   AGE
      cluster1-pxc-0                                    1/1     Running   0          5m
      cluster1-pxc-1                                    1/1     Running   0          4m
      cluster1-pxc-2                                    1/1     Running   0          2m
      cluster1-proxysql-0                               1/1     Running   0          5m
      percona-xtradb-cluster-operator-dc67778fd-qtspz   1/1     Running   0          6m

7. Check connectivity to newly created cluster

   .. code:: bash

      $ oc run -i --rm --tty percona-client --image=percona:5.7 --restart=Never -- bash -il
      percona-client:/$ mysql -h cluster1-proxysql -uroot -proot_password
