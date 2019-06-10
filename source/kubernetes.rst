.. _kubernetes:

Install the |Percona Operator PXC| on Kubernetes
============================================

0. First of all, clone the percona-xtradb-cluster-operator repository:

   .. code-block:: bash

      git clone -b release-1.0.0 https://github.com/percona/percona-xtradb-cluster-operator
      cd percona-xtradb-cluster-operator

   **Note:** *You must specify the correct branch with ``-b``
   option while cloning the code on this step.*

1. Now Custom Resource Definition for |Percona XtraDB Cluster| should be created from 
   :file:`deploy/crd.yaml`. Custom Resource Definition extends the
   standard set of resources which Kubernetes “knows” about with the new
   items (in our case ones which are the core of the operator).

   This step should be done only once; it does not need to be repeated
   with the next Operator deployments, etc.

   .. code-block:: bash

      $ kubectl apply -f deploy/crd.yaml

2. The next thing to do is to add the ``pxc`` namespace to Kubernetes,
   not forgetting to set the correspondent context for further steps:

   .. code-block:: bash

      $ kubectl create namespace pxc
      $ kubectl config set-context $(kubectl config current-context) --namespace=pxc

3. The role-based access control (RBAC) for |Percona XtraDB Cluster| is set up from
   :file:``deploy/rbac.yaml``. Briefly, role-based access is
   based on specifically defined roles and actions corresponding to
   them, allowed to be done on specific Kubernetes resources (details
   about users and roles can be found in `Kubernetes
   documentation <https://kubernetes.io/docs/reference/access-authn-authz/rbac/#default-roles-and-role-bindings>`__).

   .. code-block:: bash

      $ kubectl apply -f deploy/rbac.yaml

   **Note:** *Setting RBAC requires your user to have cluster-admin role
   privileges. For example, those using Google Kubernetes Engine can
   grant user needed privileges with the following command:*
   ``$ kubectl create clusterrolebinding cluster-admin-binding --clusterrole=cluster-admin --user=$(gcloud config get-value core/account)``

   Finally it’s time to start the operator within Kubernetes:

   .. code-block:: bash

      $ kubectl apply -f deploy/operator.yaml

4. Now that’s time to add the |Percona XtraDB Cluster| Users secrets to Kubernetes. They
   should be placed in the data section of :file:``deploy/secrets.yaml``
   as logins and base64-encoded passwords for the user accounts
   (see `Kubernetes
   documentation <https://kubernetes.io/docs/concepts/configuration/secret/>`__
   for details).

   **Note:** *the following command decodes the base64-encoded
   password:*
   ``$ echo -n 'plain-text-password' | base64``

   After editing the user name and password information the user secrets are created (or
   updated) with the following command:

   .. code-block:: bash

      $ kubectl apply -f deploy/secrets.yaml

  .. seealso::
      
      For more information, see Users_.

      .. _Users: https://www.percona.com/doc/kubernetes-operator-for-pxc/users.html

5. After the operator is started and user secrets are added, |Percona XtraDB Cluster| can be created at any time with the following command:

   .. code-block:: bash

      $ kubectl apply -f deploy/cr.yaml

   The creation process will take time. The process is over when both
   operator and cluster pod have reached their ``Running`` status:

   .. code-block:: bash

      $ kubectl get pods
      NAME                                              READY   STATUS    RESTARTS   AGE
      cluster1-pxc-node-0                               1/1     Running   0          5m
      cluster1-pxc-node-1                               1/1     Running   0          4m
      cluster1-pxc-node-2                               1/1     Running   0          2m
      cluster1-pxc-proxysql-0                           1/1     Running   0          5m
      percona-xtradb-cluster-operator-dc67778fd-qtspz   1/1     Running   0          6m

6. Check connectivity to newly created cluster

   .. code-block:: bash

      $ kubectl run -i --rm --tty percona-client --image=percona:5.7 --restart=Never -- bash -il
      percona-client:/$ mysql -h cluster1-pxc-proxysql -uroot -proot_password


.. |Percona Operator PXC| replace:: *Percona Kubernetes Operator for Percona XtraDB Cluster*
