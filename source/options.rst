Changing MySQL Options
======================

You may require a configuration change for your application. MySQL
allows the option to configure the database with a configuration file.
You can pass the MySQL options from the
`my.cnf <https://dev.mysql.com/doc/refman/8.0/en/option-files.html>`__
configuration file to the cluster in one of the following ways: \*
CR.yaml \* ConfigMap

Edit the CR.yaml
----------------

You can add options from the
`my.cnf <https://dev.mysql.com/doc/refman/8.0/en/option-files.html>`__
by editing the configuration section of the deploy/cr.yaml.

::

   spec:
     secretsName: my-cluster-secrets
     pxc:
       ...
         configuration: |
           [mysqld]
           wsrep_debug=ON
           [sst]
           wsrep_debug=ON

See the `Custom Resource options, PXC
section <https://percona.github.io/percona-xtradb-cluster-operator/configure/operator.html>`__
for more details

Use a ConfigMap
---------------

You can use a configmap and the cluster restart to reset configuration
options. A configmap allows Kubernetes to pass or update configuration
data inside a containerized application.

Use the ``kubectl`` command to create the configmap from external
resources, for more information see `Configure a Pod to use a
ConfigMap <https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/#create-a-configmap>`__.

For example, let’s suppose that your application requires more
connections. To increase your ``max_connections`` setting in MySQL, you
define a ``my.cnf`` configuration file with the following setting:

::

   [mysqld]
   ...
   max_connections=250

You can create a configmap from the ``my.cnf`` file with the
``kubectl create configmap`` command.

You should use the combination of the cluster name with the ``-pxc``
suffix as the naming convention for the configmap. To find the cluster
name, you can use the following command:

.. code:: bash

   kubectl get pxc

The syntax for ``kubectl create configmap`` command is:

::

   kubectl create configmap <cluster-name>-pxc <resource-type=resource-name>

The following example defines cluster1-pxc as the configmap name and the
my-cnf file as the data source:

.. code:: bash

   kubectl create configmap cluster1-pxc --from-file=my.cnf

To view the created configmap, use the following command:

.. code:: bash

   kubectl describe configmaps cluster1-pxc

Make changed options visible to the Percona XtraDB Cluster
----------------------------------------------------------

Do not forget to restart Percona XtraDB Cluster to ensure the cluster
has updated the configuration (see details on how to connect in the
`Install Percona XtraDB Cluster on Kubernetes
page. <https://percona.github.io/percona-xtradb-cluster-operator/install/kubernetes>`__).
