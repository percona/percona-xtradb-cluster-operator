Install Percona XtraDB Cluster cluster-wide
============================================

By default, Percona XtraDB Cluster Operator functions in specific Kubernetes
namespace - either one created as an installation step (like it is shown in the 
:ref:`installation instructions<install-kubernetes>`) or just in the ``default``
namespace. Still, there are use cases when it is highly useful to have a single
operator for multiple namespaces.

To use the Operator in such **cluster-wide** mode you should install it with a
different set of configuration YAML files, are available in the ``deploy``
folder and have filenames with a special ``cw-`` prefix:
``deploy/cw-bundle.yaml`` (the analogue of the ``deploy/bundle.yaml`` used for
 simplified installation), and others.
 
 The following example steps are showing how to install Operator cluster-wide on
 Kubernetes, similarly to our :ref:`original Kubernetes installation guide<install-kubernetes>`.

#. First of all, clone the percona-xtradb-cluster-operator repository:

   .. code:: bash

      git clone -b v{{{release}}} https://github.com/percona/percona-xtradb-cluster-operator
      cd percona-xtradb-cluster-operator

   .. note:: It is crucial to specify the right branch with ``-b``
      option while cloning the code on this step. Please be careful.

#. Now Custom Resource Definition for PXC should be created from the
   ``deploy/crd.yaml`` file. Custom Resource Definition extends the
   standard set of resources which Kubernetes “knows” about with the new
   items (in our case ones which are the core of the operator).

   This step should be done only once; it does not need to be repeated
   with the next Operator deployments, etc.

   .. code:: bash

      $ kubectl apply -f deploy/crd.yaml

#. The next thing to do is to decide which Kubernetes namespaces the Operator
   should control and in which namespace should it reside. Let's suppose that
   Operator's namespace should be the ``pxc-operator`` one. It is necessary to
   create it:

   .. code:: bash

      $ kubectl create namespace pxc-operator

   Namespaces to be under the Operator's control should be created in a same
   way, if not exist:

   .. code:: bash

      $ kubectl create namespace pxc

#. Now RBAC (role-based access control) for PXC should be set up from
   the ``deploy/cw-rbac.yaml`` file. Briefly speaking, role-based access is
   based on specifically defined roles and actions corresponding to
   them, allowed to be done on specific Kubernetes resources (details
   about users and roles can be found in `Kubernetes
   documentation <https://kubernetes.io/docs/reference/access-authn-authz/rbac/#default-roles-and-role-bindings>`_).
   
   One option in this file which may need editing is ``subjects.namespace``.
   It should contain the name of a namespace in which the Operator resides. By
   default it is equal to ``pxc-operator``. Set it to proper value if you have
   chosen a different name on the previous step. 
   
   Apply the ``deploy/cw-rbac.yaml`` file in the ``pxc-operator`` namespace
   (or in some other namespace you've chosen for the Operator to reside in) with
   the following command:

   .. code:: bash

      $ kubectl apply -f deploy/cw-rbac.yaml -n pxc-operator

   .. note:: Setting RBAC requires your user to have cluster-admin role
      privileges. For example, those using Google Kubernetes Engine can
      grant user needed privileges with the following command:
      ``$ kubectl create clusterrolebinding cluster-admin-binding --clusterrole=cluster-admin --user=$(gcloud config get-value core/account)``

#. Finally it’s time to start the operator within Kubernetes. Before doing this,
   you should specify in the ``deploy/cw-operator.yaml`` file which namespaces
   the Operator will control. in the ``env`` scection of this file, set the
   ``WATCH_NAMESPACE`` key-value pair:
   
   * if ``value`` contains empty string, the Operator will control all
     namespaces,
   * if ``value`` contains the string with a coma-separated list of the 
     namespace names, the Operator will control only namespaces from this list.

   When the editing is done, apply this file with the following command:

   .. code:: bash

      $ kubectl apply -f deploy/cw-operator.yaml -n pxc-operator

#. Now that’s time to add the PXC Users secrets to Kubernetes. This should be
   done non in the Operator's namespace, but in one we have chosen for Percona
   XtraDB Cluster (``pxc`` in our examples). 
   
   PXC Users secrets should be placed in the data section of the
   ``deploy/secrets.yaml`` file as logins and base64-encoded passwords for the
   user accounts (see `Kubernetes documentation <https://kubernetes.io/docs/concepts/configuration/secret/>`_
   for details).

   .. note:: the following command can be used to get base64-encoded
      password from a plain text string:
      ``$ echo -n 'plain-text-password' | base64``

   After editing is finished, users secrets should be created (or
   updated with the new passwords) using the following command:

   .. code:: bash

      $ kubectl apply -f deploy/secrets.yaml -n pxc

   More details about secrets can be found in :ref:`users`.

#. Now certificates should be generated. By default, the Operator generates
   certificates automatically, and no actions are required at this step. Still,
   you can generate and apply your own certificates as secrets according
   to the :ref:`TLS instructions <tls>`.

#. After the operator is started and user secrets are added, Percona
   XtraDB Cluster can be created at any time with the following command:

   .. code:: bash

      $ kubectl apply -f deploy/cr.yaml -n pxc

   Creation process will take some time. The process is over when both
   operator and replica set pod have reached their Running status:

   .. code:: bash

      $ kubectl get pods
      NAME                                              READY   STATUS    RESTARTS   AGE
      cluster1-pxc-0                                    1/1     Running   0          5m
      cluster1-pxc-1                                    1/1     Running   0          4m
      cluster1-pxc-2                                    1/1     Running   0          2m
      cluster1-proxysql-0                               1/1     Running   0          5m
      percona-xtradb-cluster-operator-dc67778fd-qtspz   1/1     Running   0          6m

#. Check connectivity to newly created cluster

   .. code:: bash

      $ kubectl run -i --rm --tty percona-client --image=percona:5.7 --restart=Never -- bash -il
      percona-client:/$ mysql -h cluster1-proxysql -uroot -proot_password


