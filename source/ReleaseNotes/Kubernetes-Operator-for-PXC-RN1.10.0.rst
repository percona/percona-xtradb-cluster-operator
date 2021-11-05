.. _K8SPXC-1.10.0:

================================================================================
*Percona Kubernetes Operator for Percona XtraDB Cluster* 1.10.0
================================================================================

:Date: November 18, 2021
:Installation: `Installing Percona Kubernetes Operator for Percona XtraDB Cluster <https://www.percona.com/doc/kubernetes-operator-for-psmongodb/index.html#installation>`_

New Features
================================================================================

* :jirabug:`K8SPXC-856`: support for defining volumes for sidecars (Thanks to Sridhar L for reporting this issue)



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

* :jirabug:`K8SPXC-761`: HAProxy container not setting explicit USER id, breaks runAsNonRoot security policy by default (Thanks to Henno Schooljan for reporting this issue)
* :jirabug:`K8SPXC-894`: Trailing white spaces cause reconcile loop on OpenShift
* :jirabug:`K8SPXC-831`: Split brain due DNS failure
* :jirabug:`K8SPXC-796`: S3 backup deletion doesn't delete Pods (Thanks to Ben Langfeld for reporting this issue)
* :jirabug:`K8SPXC-876`: ${clustername}-pxc-unready not published (Thanks to Antoine Habran for reporting this issue)
* :jirabug:`K8SPXC-842`: Backup finalizer does not delete data from S3 if folder is specified (Thanks to 申祥瑞 for reporting this issue)
* :jirabug:`K8SPXC-812`: Operator doesn't support cert-manager since v0.14.0 (Thanks to Ben Langfeld for reporting this issue)
* :jirabug:`K8SPXC-762`: Validating webhook not accepting scale operation (Thanks to Henno Schooljan for reporting this issue)
* :jirabug:`K8SPXC-893`: HAProxy pods fail during the config validation check
* :jirabug:`K8SPXC-890`: operator tries to add SYSTEM_USER privilege on 5.7 for monitor user
* :jirabug:`K8SPXC-883`: Deprecated API admissionregistration.k8s.io/v1beta1
* :jirabug:`K8SPXC-871`: Cannot to remove PXC manual backup for PVC storage
* :jirabug:`K8SPXC-851`: Changing replication user password does not work
* :jirabug:`K8SPXC-850`: Weight is not set by default for a host in a replication channel
* :jirabug:`K8SPXC-845`: Using malformed cr.yaml leads to stuck cases
* :jirabug:`K8SPXC-838`: Compute requests are inherited by log containers
* :jirabug:`K8SPXC-824`: Cluster may get into an unrecoverable state with incomplete full crash
* :jirabug:`K8SPXC-818`: pods not restarted if custom config is updated inside secret or configmap
* :jirabug:`K8SPXC-783`: Do not allow 'root@%' user to modify the monitor/clustercheck users
* :jirabug:`K8SPXC-822`: Logrotate tries to rotate GRA logs


