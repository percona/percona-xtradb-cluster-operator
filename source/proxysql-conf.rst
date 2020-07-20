.. proxysql-conf::

Configuring Load Balancing with ProxySQL
========================================

Percona XtraDB Cluster Operator provides a choice of two cluster components to
provide load balancing and proxy service: you can use either `HAProxy <https://haproxy.org>`_ or `ProxySQL <https://proxysql.com/>`_.
You can control which one to use, if any, by enabling or disabling via the
``haproxy.enabled`` and ``proxysql.enabled`` options in the ``deploy/cr.yaml``
configuration file. 

Use the following command to enable ProxySQL:

.. code:: bash

   kubectl patch pxc cluster1 --type=merge --patch '{
     "spec": {
        "proxysql": {
           "enabled": true,
           "size": 3,
           "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-proxysql" },
        "haproxy": { "enabled": false }
     }}'


.. note:: For obvious reasons the Operator will not allow the simultaneous
   enabling of both HAProxy and ProxySQL.

The resulting setup will use the number zero PXC member (``cluster1-pxc-0``
by default) as writer.

When a cluster with ProxySQL is upgraded, the following steps
take place. First, reader members are upgraded one by one: the Operator waits
until the upgraded member shows up in ProxySQL with online status, and then
proceeds to upgrade the next member. When the upgrade is finished for all
the readers, then the writer PXC member is finally upgraded.

.. note:: when both ProxySQL and PXC are upgraded, they are upgraded
   in parallel.

Accessing the ProxySQL Admin Interface
--------------------------------------

You can use `ProxySQL admin interface <https://www.percona.com/blog/2017/06/07/proxysql-admin-interface-not-typical-mysql-server/>`_ to  configure its settings.

Configuring ProxySQL in this way means connecting to it using the MySQL
protocol, and two things are needed to do it:

* the ProxySQL Pod name
* the ProxySQL admin password

You can find out ProxySQL Pod name with the ``kubectl get pods`` command,
which will have the following output::

  $ kubectl get pods
  NAME                                              READY   STATUS    RESTARTS   AGE
  cluster1-pxc-node-0                               1/1     Running   0          5m
  cluster1-pxc-node-1                               1/1     Running   0          4m
  cluster1-pxc-node-2                               1/1     Running   0          2m
  cluster1-proxysql-0                               1/1     Running   0          5m
  percona-xtradb-cluster-operator-dc67778fd-qtspz   1/1     Running   0          6m

The next command will print you the needed admin password::

  kubectl get secrets $(kubectl get pxc -o jsonpath='{.items[].spec.secretsName}') -o template='{{ .data.proxyadmin | base64decode }}'

When both Pod name and admin password are known, connect to the ProxySQL as
follows, substituting ``cluster1-proxysql-0`` with the actual Pod name and
``admin_password`` with the actual password::

  kubectl exec -it cluster1-proxysql-0 -- mysql -h127.0.0.1 -P6032 -uproxyadmin -padmin_password

.
