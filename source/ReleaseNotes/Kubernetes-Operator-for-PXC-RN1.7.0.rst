.. rn:: 1.6.0

================================================================================
*Percona Kubernetes Operator for Percona XtraDB Cluster* 1.7.0
================================================================================

:Date: January 14, 2021
:Installation: `Installing Percona Kubernetes Operator for Percona XtraDB Cluster <https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html#quickstart-guides>`_

New Features
================================================================================

* :jirabug:`K8SPXC-530`: :ref:`Backing up binary logs for point-in-time recovery<backups-pitr-binlog>`
* :jirabug:`K8SPXC-564`: Automatic full cluster crash recovery
* :jirabug:`K8SPXC-529`: Wait until PXC database removal on operator termination
* :jirabug:`K8SPXC-497`: Official support for :ref:`Percona Monitoring and Management (PMM) v.2<operator.monitoring>`

  .. note:: Monitoring with PMM v.1 configured according to the `unofficial instruction <https://www.percona.com/blog/2020/07/23/using-percona-kubernetes-operators-with-percona-monitoring-and-management/>`_
     will not work after the upgrade. Please switch to PMM v.2.

Improvements
================================================================================

* :jirabug:`K8SPXC-485`: :ref:`Make Pod logs from previous failure available<debug-images-logs>`
* :jirabug:`K8SPXC-389`: The ability to change ServiceType for HAProxy replicas
* :jirabug:`K8SPXC-546`: Reduce number of ConfigMap object updates from the Operator to improve performance of the cluster
* :jirabug:`K8SPXC-553`: Change default configuration of ProxySQL to WRITERS_ARE_READERS=yes to let cluster continue operating with a single node left
* :jirabug:`K8SPXC-548`: Add ability to pass custom PMM client parameters from CR
* :jirabug:`K8SPXC-512`: Allow to specify namespaces for cluster-wide operator to limit the scope (Thanks to user mgar for contribution)
* :jirabug:`K8SPXC-503`: Add possibility of specifying pxc init docker images in CR
* :jirabug:`K8SPXC-490`: Improve error message when not enough memory is set for auto-tuning
*(?)* :jirabug:`K8SPXC-447`: Commit version service directory
* :jirabug:`K8SPXC-312`: Add schema validation for Custom Resource
*(remove?)* :jirabug:`K8SPXC-510`: Adapt PXC operator for RedHat marketplace

Bugs Fixed
================================================================================

*(improvement "add liveness probe for HAproxy"?)* :jirabug:`K8SPXC-544`: haproxy stuck and not restarted (Thanks to user pservit for reporting this issue)
*(improvement "add the HAProxy custom config (configmap) validation"?)* :jirabug:`K8SPXC-543`: Removal haproxy custom configuration not synced with configmap (Thanks to user pservit for reporting this issue)

* :jirabug:`K8SPXC-500`: Fix a bug which prevented creating backup in cluster-wide mode (Thanks to user JIRAUSER15610 for reporting this issue)
*(make private?)* :jirabug:`K8SPXC-491`: Fix a bug due to which compressed backups didn't work with the Operator 1.6.0 (Thanks to user JIRAUSER15542 for reporting this issue)
* :jirabug:`K8SPXC-570`: Fix a bug making Minio client in backup image not mounting S3-compatible storage (Thanks to user JIRAUSER16002 for reporting this issue)
* :jirabug:`K8SPXC-517`: Fix a bug causing Operator crash if Custom Resource backup section missing (Thanks to user JIRAUSER15641 for reporting this issue)
*(make private?)* :jirabug:`K8SPXC-253`: Fix a bug preventing rolling out Custom Resource changes (Thanks to user bitsbeats for reporting this issue)
* :jirabug:`K8SPXC-499`: Fix a bug in the primary Pod detection in cluster-wide mode with HAProxy enabled
* :jirabug:`K8SPXC-552`: Fix a bug preventing correct update/sync of secrets in case of HAProxy deployment
* :jirabug:`K8SPXC-551`: Fix a bug due to which cluster was not initialized correctly with a line end in secret.yaml passwords
*(remove?)* :jirabug:`K8SPXC-537`: validationwebhook denied the request unknown field "accessModes"
* :jirabug:`K8SPXC-526`: Fix a bug due to which not all clusters managed by the Operator were upgraded by the automatic update
* :jirabug:`K8SPXC-523`: Fix a bug putting cluster into unhealthy status after clustercheck secret changed
* :jirabug:`K8SPXC-521`: Fix automatic upgrade job repeatedly looking for an already removed cluster
* :jirabug:`K8SPXC-520`: Fix Smart update in cluster-wide mode adding version service check job repeatedly instead of doing it only once
* :jirabug:`K8SPXC-463`: Fix a bug due to which wsrep_recovery log was unavailable after the Pod restart
(?)* :jirabug:`K8SPXC-424`: Fix a bug due to which HAProxy could spawn check_pxc.sh more than once making logs unreadable
* :jirabug:`K8SPXC-371`: Fix a bug making Percona XtraDB Cluster debug images not reacting on failed recovery attempt due to no sleep after the ``mysqld`` exit
* :jirabug:`K8SPXC-379`: Fix a bug due to which the Operator user credentials were not added into internal secrets when upgrading from 1.4.0 (Thanks to user pservit for reporting this issue)


Deprecation
============

* The 'serviceAccountName: percona-xtradb-cluster-operator' key was removed from ``deploy/cr.yaml`` (:jirabug:`K8SPXC-500`).
