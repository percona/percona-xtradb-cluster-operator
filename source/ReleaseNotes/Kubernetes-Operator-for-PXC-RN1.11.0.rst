.. _K8SPXC-1.11.0:

================================================================================
*Percona Operator for MySQL based on Percona XtraDB Cluster* 1.11.0
================================================================================

:Date: May 26, 2022
:Installation: `Installing Percona Operator for MySQL based on Percona XtraDB Cluster <https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html#installation>`_

Release Highlights
================================================================================

* With this release, the Operator turns to a simplified naming convention and
  changes its official name to **Percona Operator for MySQL based on Percona XtraDB Cluster**
* The new :ref:`backup.backoffLimit<backup-nackofflimit>` Custom Resource option allows customizing the number of attempts the Operator should do for backup
* The new Secrets object referenced by the :ref:`pxc.envVarsSecret<pxc-envvarssecret>` Custom Resource option can be used to pass environment variables to MySQL

New Features
================================================================================

* :jirabug:`K8SPXC-907`: Allow defining Secrets object to pass environment variables to MySQL

* :jirabug:`K8SPXC-936`: Allow modifying init script via Custom Resource, which is useful for troubleshooting the Operator’s issues


Improvements
================================================================================

* :jirabug:`K8SPXC-947`: Parametrize the number of attempt operator should do for backup
* :jirabug:`K8SPXC-738`: Labels are not applied to Service
* :jirabug:`K8SPXC-804`: Mark pxc container restarts in logs container output
* :jirabug:`K8SPXC-1009`: Enable super_read_only on replicas
* :jirabug:`K8SPXC-986`: Cleaning up users.go fo PXC privileges
* :jirabug:`K8SPXC-966`: Add ability to set annotations through helm-chart for the Operator
* :jirabug:`K8SPXC-965`: Cannot apply annotations, labels, or resource limitations to backup Pods
* :jirabug:`K8SPXC-848`: PMM container does not cause the crash of the whole database Pod if pmm-agent is not working properly
* :jirabug:`K8SPXC-758`: Allow to skip TLS verification for backup storage, useful for self-hosted S3-compatible storage with a self-issued certificate
* :jirabug:`K8SPXC-625`: Print the total number of binlogs and the number of remaining binlogs in the restore log while point-in-time recovery in progress
* :jirabug:`K8SPXC-920`: Backup Jobs Fail Intermittently (Thanks to Dustin Falgout for reporting this issue)

Bugs Fixed
================================================================================

* :jirabug:`K8SPXC-985`: Fix a bug that caused point-in-time recovery fail due to incorrect binlog filtering logic
* :jirabug:`K8SPXC-899`: Fix a bug due to which issued certificates didn't cover all hostnames, making VERIFY_IDENTITY client mode not working with HAProxy
* :jirabug:`K8SPXC-750`: Fix a bug that prevented ProxySQL from connecting to Percona XtraDB Cluster after turning TLS off
* :jirabug:`K8SPXC-896`: Fix a bug due to which the Operator was unable to create ssl-internal Secret if crash happens in the middle of a reconcile and restart (Thanks to srteam2020 for contribution)

* :jirabug:`K8SPXC-725` and :jirabug:`K8SPXC-763`: Fix a bug due to which ProxySQL StatefulSet, PVC and Services where mistakenly deleted by the Operator when reading stale ProxySQL or HAProxy information (Thanks to srteam2020 for contribution)
* :jirabug:`K8SPXC-957`: Fix a bug due to which ``pxc-db`` Helm chart didn't support setting the ``replicasServiceType`` Custom Resource option (Thanks to Carlos Martell for reporting this issue)
* :jirabug:`K8SPXC-534`: Fix a bug that caused some SQL queries to fail during the pxc StatefulSet update (Thanks to Sergiy Prykhodko for reporting this issue)
* :jirabug:`K8SPXC-1016`: Fix a bug due to which empty SSL secret name in Custom Resource made an error message to appear in the Operator log
* :jirabug:`K8SPXC-994`: get-pxc-state uses root connection
* :jirabug:`K8SPXC-961`: Fix a bug due to which a user-defined sidecar container image in the Operator Pod could be treated as the initImage (Thanks to Carlos Martell for reporting this issue)
* :jirabug:`K8SPXC-934`: Fix a bug due to which the Operator didn't create users Secret if the 'secretsName' option was absent in cr.yaml, making the cluster unable to start
* :jirabug:`K8SPXC-926`: Fix a bug due to which failed Smart Update for one cluster in cluster-wide made the Operator unusable for other clusters
* :jirabug:`K8SPXC-900`: Fix a bug that caused setting the ``--reload`` startup being ignored by ProxySQL cluster
* :jirabug:`K8SPXC-862`: Fix a bug due to which changing resources as integer values without quotes in Custom Resource could lead to cluster getting stuck
* :jirabug:`K8SPXC-858`: Fix a bug which could cause a single-node cluster Error status during upgrading
* :jirabug:`K8SPXC-814`: missing CR status when invalid option specified
* :jirabug:`K8SPXC-687`: restore not starting after failed restore on another cluster

Supported Platforms
================================================================================

The following platforms were tested and are officially supported by the Operator
1.11.0:

* OpenShift 4.7 - 4.9
* Google Kubernetes Engine (GKE) 1.19 - 1.22
* Amazon Elastic Kubernetes Service (EKS) 1.17 - 1.21
* Minikube 1.22

This list only includes the platforms that the Percona Operators are specifically tested on as part of the release process. Other Kubernetes flavors and versions depend on the backward compatibility offered by Kubernetes itself.


