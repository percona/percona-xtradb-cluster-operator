.. _operator-replication:

Set up Percona XtraDB Cluster cross-site replication
====================================================

 automates the configuration of Source and Replica Percona XtraDB Clusters in Kubernetes. But we keep in mind that either Source or Replica can run outside of Kubernetes and be out of Operators’ control. In such a case the feature will still work.

 .. image:: ./assets/images/pxc-replication.*
   :align: center

.. note:: Cross-site replication is based on `Automatic Asynchronous Replication Connection Failover<https://dev.mysql.com/doc/refman/8.0/en/replication-asynchronous-connection-failover.html>`_. Therefore it requires  MySQL 8.0 (Percona XtraDB Cluster 8.0) to work.

The full process of setting up the replica AND primary
Describe how to stop/start replication
Describe how to perform a failover
Describe that new replication user is created (in system users doc and replication doc)

How to configure replication on Source and Replica for the cluster controlled by the Operator?

You can configure cross-site replication with ``spec.pxc.replicationChannels`` section in the ``deploy/cr.yaml`` configuration file.


Example Source:

.. code:: yaml

   spec:
     pxc:
       replicationChannels:
       - name: pxc1_to_pxc2
         isSource: true


Example Replica:

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

.. _operator-replication-expose:

Exposing instances of Percona XtraDB Cluster
--------------------------------------------

You need to expose every Percona XtraDB Cluster node of the *Source* cluster to
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
   Cluster node.
