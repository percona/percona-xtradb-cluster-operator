.. _proxysql-conf:

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

The resulting setup will use the number zero Percona XtraDB Cluster member
(``cluster1-pxc-0`` by default) as writer.

When a cluster with ProxySQL is upgraded, the following steps
take place. First, reader members are upgraded one by one: the Operator waits
until the upgraded member shows up in ProxySQL with online status, and then
proceeds to upgrade the next member. When the upgrade is finished for all
the readers, then the writer Percona XtraDB Cluster member is finally upgraded.

.. note:: when both ProxySQL and Percona XtraDB Cluster are upgraded, they are
   upgraded in parallel.

.. _proxysql-conf-custom:

Passing custom configuration options to ProxySQL
------------------------------------------------

You can pass custom configuration to ProxySQL using the ``proxysql.configuration``
key in the ``deploy/cr.yaml`` file. 

.. note:: If you specify a custom ProxySQL configuration in this way, the
   Operator doesn't provide its own ProxySQL configuration file. That's why you
   should specify either a full set of configuration options or nothing.

Here is an example of ProxySQL configuration passed through ``deploy/cr.yaml``:

.. code:: yaml

   ...
   proxysql:
     enabled: false
     size: 3
     image: percona/percona-xtradb-cluster-operator:{{{release}}}-proxysql
     configuration: |
       datadir="/var/lib/proxysql"

       admin_variables =
       {
         admin_credentials="proxyadmin:admin_password"
         mysql_ifaces="0.0.0.0:6032"
         refresh_interval=2000

         cluster_username="proxyadmin"
         cluster_password="admin_password"
         cluster_check_interval_ms=200
         cluster_check_status_frequency=100
         cluster_mysql_query_rules_save_to_disk=true
         cluster_mysql_servers_save_to_disk=true
         cluster_mysql_users_save_to_disk=true
         cluster_proxysql_servers_save_to_disk=true
         cluster_mysql_query_rules_diffs_before_sync=1
         cluster_mysql_servers_diffs_before_sync=1
         cluster_mysql_users_diffs_before_sync=1
         cluster_proxysql_servers_diffs_before_sync=1
       }

       mysql_variables=
       {
         monitor_password="monitor"
         monitor_galera_healthcheck_interval=1000
         threads=2
         max_connections=2048
         default_query_delay=0
         default_query_timeout=10000
         poll_timeout=2000
         interfaces="0.0.0.0:3306"
         default_schema="information_schema"
         stacksize=1048576
         connect_timeout_server=10000
         monitor_history=60000
         monitor_connect_interval=20000
         monitor_ping_interval=10000
         ping_timeout_server=200
         commands_stats=true
         sessions_sort=true
         have_ssl=true
         ssl_p2s_ca="/etc/proxysql/ssl-internal/ca.crt"
         ssl_p2s_cert="/etc/proxysql/ssl-internal/tls.crt"
         ssl_p2s_key="/etc/proxysql/ssl-internal/tls.key"
         ssl_p2s_cipher="ECDHE-RSA-AES128-GCM-SHA256"
       }
   ...

.. _proxysql-conf-admin:

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
