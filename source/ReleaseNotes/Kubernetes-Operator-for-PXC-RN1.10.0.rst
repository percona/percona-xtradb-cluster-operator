.. _K8SPXC-1.10.0:

================================================================================
*Percona Kubernetes Operator for Percona XtraDB Cluster* 1.10.0
================================================================================

:Date: November 18, 2021
:Installation: `Installing Percona Kubernetes Operator for Percona XtraDB Cluster <https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html#quickstart-guides>`_

Release Highlights
================================================================================


New Features
================================================================================

* :jirabug:`K8SPXC-856`: support for defining volumes for sidecars (Thanks to Sridhar L for contributing)



Improvements
================================================================================

* :jirabug:`K8SPXC-889`: Add more details about Local Storage usage to documentation
* :jirabug:`K8SPXC-771`: Expose all fields supported in the CRD to the Helm chart for PXC-DB (Thanks to Gerwin van de Steeg for reporting this issue)
* :jirabug:`K8SPXC-794`: Flood of rotate information in logs
* :jirabug:`K8SPXC-793`: Logs are very messy
* :jirabug:`K8SPXC-789`: DR Replication - tune master retries for replication between two clusters
* :jirabug:`K8SPXC-588`: Allow disabling k8s service for haproxy-replicas with a flag



Bugs Fixed
================================================================================

* :jirabug:`K8SPXC-761`: Fixed a bug where HAProxy container was not setting explicit USER id, being incompatible with the runAsNonRoot security policy (Thanks to Henno Schooljan for reporting this issue)
* :jirabug:`K8SPXC-894`: Fixed a bug where trailing white spaces in the ``pmm-admin add command`` command caused reconcile loop on OpenShift
* :jirabug:`K8SPXC-831`: Fixed a bug which made it possible to have a split brain situation, when two nodes were starting their own cluster in case of a DNS failure
* :jirabug:`K8SPXC-796`: Fixed a bug due to which S3 backup deletion didn't delete Pods attached to the backup job if the S3 finalizer was set (Thanks to Ben Langfeld for reporting this issue)
* :jirabug:`K8SPXC-876`: ${clustername}-pxc-unready not published (Thanks to Antoine Habran for reporting this issue)
* :jirabug:`K8SPXC-842`: Fixed a bug where backup finalizer didn't delete data from S3 if backup path contained a folder inside of the S3 bucket (Thanks to 申祥瑞 for reporting this issue)
* :jirabug:`K8SPXC-812`: Fix a bug due to which the Operator didn't support cert-manager versions since v0.14.0 (Thanks to Ben Langfeld for reporting this issue)
* :jirabug:`K8SPXC-762`: Fix a bug due to which the validating webhook was not accepting scale operation in the Operator cluster-wide mode (Thanks to Henno Schooljan for reporting this issue)
* :jirabug:`K8SPXC-893`: Fix a bug where HAProxy service failed during the config validation check if there was a resolution fail with one fo the PXC addresses
* :jirabug:`K8SPXC-871`: Fix a bug which prevented removing Percona a XtraDB Cluster manual backup for PVC storage
* :jirabug:`K8SPXC-851`: Fixed a bug where changing replication user password didn't work
* :jirabug:`K8SPXC-850`: Fixed a bug where the default weight value wasn't set for a host in a replication channel
* :jirabug:`K8SPXC-845`: Fixed a bug where using malformed cr.yaml caused stuck cases in cluster deletion
* :jirabug:`K8SPXC-838`: Compute requests are inherited by log containers
* :jirabug:`K8SPXC-824`: Cluster may get into an unrecoverable state with incomplete full crash
* :jirabug:`K8SPXC-818`: pods not restarted if custom config is updated inside secret or configmap
* :jirabug:`K8SPXC-783`: Do not allow 'root@%' user to modify the monitor/clustercheck users
* :jirabug:`K8SPXC-822`: Logrotate tries to rotate GRA logs

Deprecation and Removal
================================================================================

* We are simplifying the way the user can customize MongoDB components such as
  mongod and mongos. :ref:`It is now possible<operator-configmaps>`
  to set custom configuration through ConfigMaps and Secrets Kubernetes
  resources. The following options will be deprecated in Percona Distribution
  for MongoDB Operator v1.9.0+, and completely removed in v1.12.0+:

  * ``sharding.mongos.auditLog.*``
  * ``mongod.security.redactClientLogData``
  * ``mongod.security.*``
  * ``mongod.setParameter.*``
  * ``mongod.storage.*``
  * ``mongod.operationProfiling.mode``
  * ``mongod.auditLog.*``
* The mongos.expose.enabled option has been completely removed from the Custom
  Resource as it was causing confusion for the users


