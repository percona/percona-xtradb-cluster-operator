.. rn:: 1.6.0

================================================================================
*Percona Kubernetes Operator for Percona XtraDB Cluster* 1.7.0
================================================================================

:Date: January 14, 2021
:Installation: `Installing Percona Kubernetes Operator for Percona XtraDB Cluster <https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html#quickstart-guides>`_

New Features
================================================================================

* :jirabug:`K8SPXC-530`: PITR - Recovery part
* :jirabug:`K8SPXC-529`: Wait until PXC database removal on operator termination
* :jirabug:`K8SPXC-482`: [PXC] Create Binary Log collector

Improvements
================================================================================

* :jirabug:`K8SPXC-389`: The ability to change ServiceType for HAProxy replicas
* :jirabug:`K8SPXC-546`: Reduce number of ConfigMap object updates from the Operator to improve performance of the cluster
*(remove?)* :jirabug:`K8SPXC-485`: Make pod logs from previous failure available
* :jirabug:`K8SPXC-553`: Change default configuration of ProxySQL to WRITERS_ARE_READERS=yes to let cluster continue operating with a single node left
*(o)* :jirabug:`K8SPXC-548`: Add ability to pass custom PMM client parameters from CR
*(&)* :jirabug:`K8SPXC-512`: Allow to specify namespaces for cluster-wide operator to limit the scope (Thanks to user JIRAUSER15637 for reporting this issue)
* :jirabug:`K8SPXC-472`: Update k8s-vault-issuer to load Vault token from file (Thanks to user john.schaeffer for reporting this issue)
* :jirabug:`K8SPXC-564`: Automatic full cluster crash recovery
*(o)* :jirabug:`K8SPXC-503`: Add possibility of specifying pxc init docker images in CR
*(&)* :jirabug:`K8SPXC-497`: Fix PMM and PXC operator integration
* :jirabug:`K8SPXC-490`: Improve error message when not enough memory is set for auto-tuning
*(?)* :jirabug:`K8SPXC-447`: Commit version service directory
* :jirabug:`K8SPXC-312`: Add schema validation for Custom Resource
*(remove?)* :jirabug:`K8SPXC-510`: Adapt PXC operator for RedHat marketplace

Bugs Fixed
================================================================================

* :jirabug:`K8SPXC-500`: Unable to create backup when using cluster-wide operator (Thanks to user JIRAUSER15610 for reporting this issue)
*(make private?)* :jirabug:`K8SPXC-491`: Compressed backups not working with the Operator 1.6.0 (Thanks to user JIRAUSER15542 for reporting this issue)
* :jirabug:`K8SPXC-570`: Minio client in backup image does not mount S3-compatible storage (Thanks to user JIRAUSER16002 for reporting this issue)
*(improvement?)* :jirabug:`K8SPXC-544`: haproxy stuck and not restarted *(add liveness probe for HAproxy)* (Thanks to user pservit for reporting this issue)
*(improvement?)* :jirabug:`K8SPXC-543`: Removal haproxy custom configuration not synced with configmap *(added the for HAProxy custom config (configmap) validation)* (Thanks to user pservit for reporting this issue)
* :jirabug:`K8SPXC-517`: Operator 1.6.0 crash if the Custom Resource backup section missing (Thanks to user JIRAUSER15641 for reporting this issue)
*(make private?)* :jirabug:`K8SPXC-253`: Changes on CR are not rolled out (Thanks to user bitsbeats for reporting this issue)
* :jirabug:`K8SPXC-499`: fix primary Pod detection in cluster-wide mode if HAProxy enabled
* :jirabug:`K8SPXC-552`: The secrets not updated/synced correctly in case of HAProxy deployment
* :jirabug:`K8SPXC-551`: Cluster not initialized correctly with line end in secret.yaml passwords
*(remove?)* :jirabug:`K8SPXC-537`: validationwebhook denied the request unknown field "accessModes"
* :jirabug:`K8SPXC-526`: Fix a bug due to which not all clusters managed by the Operator were upgraded by the automatic update
* :jirabug:`K8SPXC-523`: Cluster going into unhealthy status after clustercheck secret changed
* :jirabug:`K8SPXC-521`: Automatic upgrade job is repeatedly looking for an already removed cluster
(?)* :jirabug:`K8SPXC-520`: Smart update in cluster-wide mode adds version service check job repeatedly instead of doing it only once
* :jirabug:`K8SPXC-463`: wsrep_recovery log unavailable after the Pod restart
(?)* :jirabug:`K8SPXC-424`: Haproxy can spawn check_pxc.sh more than once that makes logs unreadable
* :jirabug:`K8SPXC-371`: Percona XtraDB Cluster debug images not reacting on failed recovery attempt due to no sleep after the ``mysqld`` exit
* :jirabug:`K8SPXC-379`: The Operator user credentials not added into internal secrets when upgrading from 1.4.0 (Thanks to user pservit for reporting this issue)
