.. _operator-configmaps:

Changing MySQL Options
======================

You may require a configuration change for your application. MySQL
allows the option to configure the database with a configuration file.
You can pass options from the
`my.cnf <https://dev.mysql.com/doc/refman/8.0/en/option-files.html>`__
configuration file to be included in the MySQL configuration in one of the
following ways:

* edit the ``deploy/cr.yaml`` file,
* use a ConfigMap,
* use a Secret object.

.. _operator-configmaps-cr:

Edit the ``deploy/cr.yaml`` file
---------------------------------

You can add options from the
`my.cnf <https://dev.mysql.com/doc/refman/8.0/en/option-files.html>`__
configuration file by editing the configuration section of the
``deploy/cr.yaml``. Here is an example:

.. code:: yaml

   spec:
     secretsName: my-cluster-secrets
     pxc:
       ...
         configuration: |
           [mysqld]
           wsrep_debug=CLIENT
           [sst]
           wsrep_debug=CLIENT

See the `Custom Resource options, PXC
section <operator.html#operator-pxc-section>`_
for more details

.. _operator-configmaps-cm:

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

The following example defines ``cluster1-pxc`` as the configmap name and the
``my.cnf`` file as the data source:

.. code:: bash

   kubectl create configmap cluster1-pxc --from-file=my.cnf

To view the created configmap, use the following command:

.. code:: bash

   kubectl describe configmaps cluster1-pxc

.. _operator-configmaps-secret:

Use a Secret Object
-------------------

The Operator can also store configuration options in `Kubernetes Secrets <https://kubernetes.io/docs/concepts/configuration/secret/>`_.
This can be useful if you need additional protection for some sensitive data.

You should create a Secret object with a specific name, composed of your cluster
name and the ``pxc`` suffix.
  
.. note:: To find the cluster name, you can use the following command:

   .. code:: bash

      $ kubectl get pxc

Configuration options should be put inside a specific key inside of the ``data``
section. The name of this key is ``my.cnf`` for Percona XtraDB Cluster Pods.

Actual options should be encoded with `Base64 <https://en.wikipedia.org/wiki/Base64>`_.

For example, let's define a ``my.cnf`` configuration file and put there a pair
of MySQL options we used in the previous example:

::

   [mysqld]
   wsrep_debug=CLIENT
   [sst]
   wsrep_debug=CLIENT

You can get a Base64 encoded string from your options via the command line as
follows:

.. code:: bash

   $ cat my.cnf | base64

.. note:: Similarly, you can read the list of options from a Base64 encoded
   string:

   .. code:: bash

      $ echo "W215c3FsZF0Kd3NyZXBfZGVidWc9T04KW3NzdF0Kd3NyZXBfZGVidWc9T04K" | base64 --decode

Finally, use a yaml file to create the Secret object. For example, you can
create a ``deploy/my-pxc-secret.yaml`` file with the following contents:

.. code:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: cluster1-pxc
   data:
     my.cnf: "W215c3FsZF0Kd3NyZXBfZGVidWc9T04KW3NzdF0Kd3NyZXBfZGVidWc9T04K"

When ready, apply it with the following command:

.. code:: bash

   $ kubectl create -f deploy/my-pxc-secret.yaml

.. note:: Do not forget to restart Percona XtraDB Cluster to ensure the
   cluster has updated the configuration.

.. _operator-configmaps-restart:

Make changed options visible to the Percona XtraDB Cluster
----------------------------------------------------------

Do not forget to restart Percona XtraDB Cluster to ensure the cluster
has updated the configuration (see details on how to connect in the
`Install Percona XtraDB Cluster on Kubernetes <kubernetes.html>`_ page).

.. _operator-configmaps-auto:

Auto-tuning MySQL options
--------------------------

Few configuration options for MySQL can be calculated and set by the Operator
automatically based on the available Pod resources (memory and CPU) **if
these options are not specified by user** (either in CR.yaml or in ConfigMap).

Options which can be set automatically are the following ones:

* ``innodb_buffer_pool_size``
* ``max_connections``

If Percona XtraDB Cluster Pod limits are defined, then limits values are used to
calculate these options. If Percona XtraDB Cluster Pod limits are not defined,
Operator looks for Percona XtraDB Cluster Pod requests as the basis for
calculations. if neither Percona XtraDB Cluster Pod limits nor Percona XtraDB
Cluster Pod requests are defined, auto-tuning is not done.
