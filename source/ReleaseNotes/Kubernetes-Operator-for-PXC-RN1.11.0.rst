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

New Features
================================================================================

* :jirabug:`K8SPXC-907`: Provide a way to use jemalloc for mysqld
* :jirabug:`K8SPXC-947`: Parametrize the number of attempt operator should do for backup
* :jirabug:`K8SPXC-936`: Hookable init scripts
* :jirabug:`K8SPXC-935`: Add possibility to specify nodePort for PXC operator for k8s



Improvements
================================================================================

* :jirabug:`K8SPXC-960`: document that both full backup and binlogs should be on S3
* :jirabug:`K8SPXC-738`: Labels are not applied to Service
* :jirabug:`K8SPXC-804`: Mark pxc container restarts in logs container output
* :jirabug:`K8SPXC-1009`: Enable super_read_only on replicas
* :jirabug:`K8SPXC-986`: Cleaning up users.go fo PXC privileges
* :jirabug:`K8SPXC-966`: Add ability to set annotations through helm-chart for operator
* :jirabug:`K8SPXC-965`: Cannot apply annotations, labels, or resource limitations to backup pods
* :jirabug:`K8SPXC-848`: pmm container should not crash in case of issues
* :jirabug:`K8SPXC-758`: Allow to skip TLS verification for backup storage
* :jirabug:`K8SPXC-625`: improve logs for PITR

Bugs Fixed
================================================================================

* :jirabug:`K8SPXC-985`: PITR fails due to incorrect binlog filtering logic
* :jirabug:`K8SPXC-899`: sql_mode=VERIFY_IDENTITY not working with HAProxy and cert-manager
* :jirabug:`K8SPXC-750`: ProxySQL can't connect to PXC if allowUnsafeConfiguration = true
* :jirabug:`K8SPXC-896`: [BUG] Operator cannot create ssl-internal secret if crash happens at some particular point (Thanks to srteam2020 for reporting this issue)
* :jirabug:`K8SPXC-763`: [BUG] Proxysql statefulset, PVC and services get mistakenly deleted when reading stale proxysql information (Thanks to srteam2020 for reporting this issue)
* :jirabug:`K8SPXC-725`: [BUG] HAproxy statefulset and services get mistakenly deleted when reading stale `spec.haproxy.enabled` (Thanks to srteam2020 for reporting this issue)
* :jirabug:`K8SPXC-957`: replicasServiceType set in helm chart not passed through to operator (Thanks to Carlos Martell for reporting this issue)
* :jirabug:`K8SPXC-920`: Backup Jobs Fail Intermittently (Thanks to Dustin Falgout for reporting this issue)
* :jirabug:`K8SPXC-534`: No servers in hostgroup 10 during pxc statefulset update (Thanks to Sergiy Prykhodko for reporting this issue)
* :jirabug:`K8SPXC-1016`: Reconciler error due to empty SSL secret name
* :jirabug:`K8SPXC-994`: get-pxc-state uses root connection
* :jirabug:`K8SPXC-961`: Operator assumes there is no other containers running on operator pod while defining initImage (Thanks to Carlos Martell for reporting this issue)
* :jirabug:`K8SPXC-934`: Create secret for system users even if 'secretsName' option is commented in CR
* :jirabug:`K8SPXC-926`: failed smart update for one cluster makes the operator unusable for other clusters
* :jirabug:`K8SPXC-900`: reload startup option not working in proxysql cluster
* :jirabug:`K8SPXC-862`: Changing resources might lead to cluster getting stuck
* :jirabug:`K8SPXC-858`: PXC cluster is in Error status during upgrading
* :jirabug:`K8SPXC-835`: proxysql errors when used in replica cluster
* :jirabug:`K8SPXC-814`: missing CR status when invalid option specified
* :jirabug:`K8SPXC-687`: restore not starting after failed restore on another cluster
* :jirabug:`K8SPXC-975`: typo `xtrabcupUser`

Supported Platforms
================================================================================

The following platforms were tested and are officially supported by the Operator
1.11.0:

* OpenShift 4.7 - 4.9
* Google Kubernetes Engine (GKE) 1.19 - 1.22
* Amazon Elastic Kubernetes Service (EKS) 1.17 - 1.21
* Minikube 1.22

This list only includes the platforms that the Percona Operators are specifically tested on as part of the release process. Other Kubernetes flavors and versions depend on the backward compatibility offered by Kubernetes itself.


