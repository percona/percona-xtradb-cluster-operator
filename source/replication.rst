.. _operator-replication:

Set up Percona XtraDB Cluster cross-site replication
====================================================

 automates the configuration of Source and Replica Percona XtraDB Clusters in Kubernetes. But we keep in mind that either Source or Replica can run outside of Kubernetes and be out of Operators’ control. In such a case the feature will still work.
 
 this feature for MySQL version 8.0 only based on `Automatic Asynchronous Replication Connection Failover<https://dev.mysql.com/doc/refman/8.0/en/replication-asynchronous-connection-failover.html>`_

 .. image:: ./assets/images/pxc-replication.*
   :align: center
describe the replication configuration
The full process of setting up the replica AND primary
Describe how to stop/start replication
Describe how to perform a failover
Describe that new replication user is created (in system users doc and replication doc)

How to configure replication on Source and Replica for the cluster controlled by the Operator?

We add the new section spec.pxc.replicationChannels


Example Source:

spec:
  pxc:
    replicationChannels:
    - name: pxc1_to_pxc2
      isSource: true
      

Example Replica:

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


For Replica cluster to connect to Source every PXC node in Source cluster should be exposed.

We are going to add a new section under spec.pxc - expose.

Example:

spec:
  pxc:
    expose:
      enabled: true
      type: LoadBalancer
      loadBalancerSourceRanges:
        - 10.0.0.0/8
      annotations: 
        networking.gke.io/load-balancer-type: "Internal"

This will create the internal LoadBalancer per each PXC node.


