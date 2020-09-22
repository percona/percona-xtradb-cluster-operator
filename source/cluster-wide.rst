Install Percona XtraDB Cluster cluster-wide
============================================

By default, Percona XtraDB Cluster Operator functions in a specific Kubernetes
namespace. You can create one during installation (like it is shown in the 
:ref:`installation instructions<install-kubernetes>`) or just use the ``default``
namespace. This approach allows several Operators to co-exist in one
Kubernetes-based environment, being separated in different namespaces.

Still, sometimes it is more convenient to have one Operator watching for
Percona XtraDB Cluster custom resources in several namespaces.

To use the Operator in such *cluster-wide* mode, you should install it with a
different set of configuration YAML files, which are available in the ``deploy``
folder and have filenames with a special ``cw-`` prefix: e.g.
``deploy/cw-bundle.yaml``.

While using this cluster-wide versions of configuration files, you should set
the following information there:

* ``subjects.namespace`` option should contain the namespace which will host
  the Operator,
* ``WATCH_NAMESPACE`` key-value pair in the ``env`` section should have
  ``value`` equal to a  comma-separated list of the namespaces to be watched by
  the Operator (or just a blank string to make the Operator deal with *all
  namespaces* in a Kubernetes cluster).

  .. note:: Technically it is possible to have several Operators with the same
     list of namespaces. But in this case, it is unpredictable, which
     Operator will get ownership of the Custom Resource in each namespace.
     Therefore, we recommend to have not more than one Operator watching each
     namespace, or to have one cluster-wide Operator watching several namespaces
     at once.

The following simple example shows how to install Operator cluster-wide on
Kubernetes.

#. First of all, clone the percona-xtradb-cluster-operator repository:

   .. code:: bash

      git clone -b v{{{release}}} https://github.com/percona/percona-xtradb-cluster-operator
      cd percona-xtradb-cluster-operator

#. The next thing to do is to decide which Kubernetes namespaces the Operator
   should control and in which namespace should it reside. Let's suppose that
   Operator's namespace should be the ``pxc-operator`` one. Create it as
   follows:

   .. code:: bash

      $ kubectl create namespace pxc-operator

   Namespaces to be watched by the Operator should be created in the same way if
   not exist. Let's say the Operator should watch the ``pxc`` namespace:

   .. code:: bash

      $ kubectl create namespace pxc

#. Edit the ``deploy/cw-bundle.yaml`` configuration file to set proper namespaces:

   .. code:: yaml

      ...
      subjects:
      - kind: ServiceAccount
        name: percona-xtradb-cluster-operator
        namespace: "pxc-operator"
      ...
      env:
               - name: WATCH_NAMESPACE
                 value: "pxc"
      ...

   When the editing is done, apply the file with the following command:

   .. code:: bash

      $ kubectl apply -f deploy/cw-bundle.yaml -n pxc-operator

#. After the Operator is started, Percona XtraDB Cluster can be created at any
   time by applying the ``deploy/cr.yaml`` configuration file, like in the case
   of normal installation:

   .. code:: bash

      $ kubectl apply -f deploy/cr.yaml -n pxc

   The creation process will take some time. The process is over when both
   operator and replica set Pods have reached their Running status:

   .. code:: bash

      $ kubectl get pods -n pxc
      NAME                                              READY   STATUS    RESTARTS   AGE
      cluster1-pxc-0                                    1/1     Running   0          5m
      cluster1-pxc-1                                    1/1     Running   0          4m
      cluster1-pxc-2                                    1/1     Running   0          2m
      cluster1-proxysql-0                               1/1     Running   0          5m

#. Check connectivity to newly created cluster

   .. code:: bash

      $ kubectl run -i --rm --tty percona-client --image=percona:5.7 --restart=Never --env="POD_NAMESPACE=pxc" -- bash -il
      percona-client:/$ mysql -h cluster1-proxysql -uroot -proot_password
