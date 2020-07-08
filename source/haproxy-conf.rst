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


