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

Passing HAProxy configuration options
-------------------------------------

You can pass options to HAProxy using the ``haproxy.configuration`` key in the
``deploy/cr.yaml`` file as follows:

.. code:: yaml

   ...
   haproxy:
    enabled: true
    size: 3
    image: percona/percona-xtradb-cluster-operator:1.5.0-haproxy
    configuration: |
      global
        maxconn 4096
      defaults
        option  dontlognull
        retries 3
        redispatch
        maxconn 2000
    ...
