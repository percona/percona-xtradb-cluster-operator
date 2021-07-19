.. _operator-replication:

Set up Percona XtraDB Cluster cross-site replication
====================================================

The cross-site replication involves configuring one Percona XtraDB Cluster as *Source*, and another Percona XtraDB Cluster as *Replica* to allow an asynchronous replication between them:

 .. image:: ./assets/images/pxc-replication.svg
   :align: center

The Operator automates configuration of *Source* and *Replica* Percona XtraDB Clusters, but the feature itself is not bound to Kubernetes. Either *Source* or *Replica* can run outside of Kubernetes and be out of the Operators’ control. 

This feature can be useful in several cases: for example, it can simplify migration from on-premises to the cloud with replication, and it can be really helpful in case of the disaster recovery too.

.. note:: Cross-site replication is based on `Automatic Asynchronous Replication Connection Failover<https://dev.mysql.com/doc/refman/8.0/en/replication-asynchronous-connection-failover.html>`_. Therefore it requires  MySQL 8.0 (Percona XtraDB Cluster 8.0) to work.

.. Describe how to stop/start replication
   Describe how to perform a failover

Setting up Percona XtraDB Cluster for asynchronous replication without the Operator is described `here <https://www.percona.com/blog/2018/03/19/percona-xtradb-cluster-mysql-asynchronous-replication-and-log-slave-updates/>`_ and is out of the scope for this document.

Configuring the cross-site replication for the cluster controlled by the Operator is explained in the following subsections.

.. contents:: :local:

.. _operator-replication-source:

Configuring cross-site replication on Source instances
------------------------------------------------------

You can configure *Source* instances for cross-site replication with ``spec.pxc.replicationChannels`` subsection in the ``deploy/cr.yaml`` configuration file. It is an array of channels, and you should provide the following keys for the channel in your *Source* Percona XtraDB Cluster:

* ``pxc.replicationChannels.[].name`` key is the name of the channel,

* ``pxc.replicationChannels.[].isSource`` key should be set to ``true``.

Here is an example:

.. code:: yaml

   spec:
     pxc:
       replicationChannels:
       - name: pxc1_to_pxc2
         isSource: true

The cluster will be ready for asynchronous replication when you apply changes as usual:

.. code:: bash

   $ kubectl apply -f deploy/cr.yaml

.. _operator-replication-replica:

Configuring cross-site replication on Replica instances
-------------------------------------------------------

You can configure *Replica* instances for cross-site replication with ``spec.pxc.replicationChannels`` subsection in the ``deploy/cr.yaml`` configuration file. It is an array of channels, and you should provide the following keys for the channel in your *Replica* Percona XtraDB Cluster:

* ``pxc.replicationChannels.[].name`` key is the name of the channel,

* ``pxc.replicationChannels.[].isSource`` key should be set to ``false``,

* ``pxc.replicationChannels.[].sourcesList`` is the list of *Source* cluster names from which Replica should get the data,

* ``pxc.replicationChannels.[].sourcesList.[].host`` is the host name or IP-address of the Source,

* ``pxc.replicationChannels.[].sourcesList.[].port`` is the port of the source (``3306`` port will be used if nothing specified),

* ``pxc.replicationChannels.[].sourcesList.[].weight`` is the *weight* of the source (``100`` by default).

Here is the example:

.. code:: yaml

   spec:
     pxc:
       replicationChannels:
       - name: uspxc1_to_pxc2
         isSource: false
         sourcesList:
         - host: pxc1.source.percona.com
           port: 3306
           weight: 100
         - host: pxc2.source.percona.com
         - host: pxc3.source.percona.com
       - name: eu_to_pxc2
         isSource: false
         sourcesList:
         - host: pxc1.source.percona.com
           port: 3306
           weight: 100
         - host: pxc2.source.percona.com
         - host: pxc3.source.percona.com

The cluster will be ready for asynchronous replication when you apply changes as usual:

.. code:: bash

   $ kubectl apply -f deploy/cr.yaml

.. _operator-replication-expose:

Exposing instances of Percona XtraDB Cluster
--------------------------------------------

You need to expose every Percona XtraDB Cluster Pod of the *Source* cluster to
make it possible for the *Replica* cluster to connect. This is done through the
``pxc.expose`` section in the ``deploy/cr.yaml`` configuration file as follows.

.. code:: yaml

   spec:
     pxc:
       expose:
         enabled: true
         type: LoadBalancer
         loadBalancerSourceRanges:
           - 10.0.0.0/8
         annotations: 
           networking.gke.io/load-balancer-type: "Internal"

.. note:: This will create the internal LoadBalancer per each Percona XtraDB
   Cluster Pod.

.. _operator-replication-user:

System user for replication
---------------------------

Replication channel demands a special :ref:`system user<users.system-users>` with same credentials on both *Source* and *Replica*.
The Operator creates a system-level Percona XtraDB Cluster user named ``replication`` for this purpose, with
credentials stored in a Secret object :ref:`along with other system users<users.system-users>`.

You can change a password for this user as follows:

.. code:: bash

   kubectl patch secret/my-cluster-name-secrets -p '{"data":{"replication": '$(echo -n new_password | base64)'}}'

If the cluster is outside of Kubernetes and is not under the Operator's control, `the appropriate user with necessary permissions <https://dev.mysql.com/doc/refman/8.0/en/replication-asynchronous-connection-failover.html>`_ should be created manually.
