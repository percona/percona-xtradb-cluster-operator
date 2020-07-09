.. haproxy-conf::

Configuring Load Balancing with HAProxy
=======================================

Percona XtraDB Cluster Operator provides choice of two cluster components to
carry on load balancing and proxy service: you can use either `HAProxy<https://haproxy.org>`_ or `ProxySQL <https://proxysql.com/>`_.
You can control which one to use, if any, enabling or disabling via the
``haproxy.enabled`` and ``proxysql.enabled`` options in the ``deploy/cr.yaml``
configuration file. 

Use the following command to enable HAProxy:

.. code:: bash

   kubectl patch pxc cluster1 --type=merge --patch '{ \
    "spec": {"haproxy":{ "enabled": true }, \
    "proxysql":{ "enabled": false } \
    }}'

.. note:: For obvious reasons the Operator will not allow the simultaneous
   enabling of both HAProxy and ProxySQL.

The resulting HAPproxy setup will contain two services:

* ``cluster1-haproxy`` service listening on ports 3306 (MySQL) and 3309 (proxy).
  This service is pointing to a zero node (``cluster1-pxc-0``) by default when
  this node is available. If zero node is not available, nodes are selected in
  descending order of their numbers (eg. ``cluster1-pxc-2``, then
  ``cluster1-pxc-1``, etc.). This service can be used for both read and write
  load, or it can also be used just for write load (single writer mode) in
  setups with split write and read loads.

* ``cluster1-haproxy-replicas`` listening on port 3306 (MySQL).
  This service selects PXC nodes to serve queries following the Round Robin
  load balancing algorithm.

Passing custom configuration options to HAProxy
-----------------------------------------------

You can pass custom configuration to HAProxy using the ``haproxy.configuration``
key in the ``deploy/cr.yaml`` file. 

.. note:: If you specify a custom HAProxy configuration in this way, the
   Operator doesn't provide its own HAProxy configuration file. That's why you
   should specify either a full set of configuration options or nothing.

Here is an example of HAProxy configuration passed through ``deploy/cr.yaml``:

.. code:: yaml

   ...
   haproxy:
       enabled: true
       size: 3
       image: percona/percona-xtradb-cluster-operator:1.5.0-haproxy
       configuration: |
         global
           maxconn 2048
           external-check
           stats socket /var/run/haproxy.sock mode 600 expose-fd listeners level user
         defaults
           log global
           mode tcp
           retries 10
           timeout client 10000
           timeout connect 100500
           timeout server 10000
         frontend galera-in
           bind *:3309 accept-proxy
           bind *:3306
           mode tcp
           option clitcpka
           default_backend galera-nodes
         frontend galera-replica-in
           bind *:3309 accept-proxy
           bind *:3307
           mode tcp
           option clitcpka
           default_backend galera-replica-nodes
    ...
