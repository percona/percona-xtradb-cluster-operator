.. _K8SPXC-1.10.0:

================================================================================
*Percona Kubernetes Operator for Percona XtraDB Cluster* 1.10.0
================================================================================

:Date: November 18, 2021
:Installation: `Installing Percona Kubernetes Operator for Percona XtraDB Cluster <https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html#quickstart-guides>`_

Release Highlights
================================================================================

* Now it is possible to define volumes for sidecar containers

New Features
================================================================================

* :jirabug:`K8SPXC-856`: support for defining volumes for sidecars (Thanks to Sridhar L for contributing)

Improvements
================================================================================

* :jirabug:`K8SPXC-771`: All Custom Resource options available with the Operator are now :ref:`exposed to the Helm chart<install-helm-params>` (Thanks to Gerwin van de Steeg for reporting this issue)
* :jirabug:`K8SPXC-794`: The ``logrotate`` command now doesn't use verbose mode to avoid flooding the log with rotate information
* :jirabug:`K8SPXC-793`: Logs are very messy
* :jirabug:`K8SPXC-789`: New :ref:`source_retry_count<pxc-replicationchannels-configuration-sourceretrycount>` and :ref:`source_connect_retry<pxc-replicationchannels-configuration-sourceconnectretry>` options were added to tune source retries for replication between two clusters
* :jirabug:`K8SPXC-588`: New :ref:`replicasServiceEnabled<haproxy-replicasserviceenabled>` option was added to allow disabling the Kubernetes Service for ``haproxy-replicas``, which may be useful to avoid the unwanted forwarding of the application write requests to all Percona XtraDB Cluster instances

Bugs Fixed
================================================================================

* :jirabug:`K8SPXC-761`: Fixed a bug where HAProxy container was not setting explicit USER id, being incompatible with the runAsNonRoot security policy (Thanks to Henno Schooljan for reporting this issue)
* :jirabug:`K8SPXC-894`: Fixed a bug where trailing white spaces in the ``pmm-admin add command`` command caused reconcile loop on OpenShift
* :jirabug:`K8SPXC-831`: Fixed a bug which made it possible to have a split brain situation, when two nodes were starting their own cluster in case of a DNS failure
* :jirabug:`K8SPXC-796`: Fixed a bug due to which S3 backup deletion didn't delete Pods attached to the backup job if the S3 finalizer was set (Thanks to Ben Langfeld for reporting this issue)
* :jirabug:`K8SPXC-876`: Stopped using the ``service.alpha.kubernetes.io/tolerate-unready-endpoints`` deprecated kubernetes option in the ``${clustername}-pxc-unready`` service annotation (Thanks to Antoine Habran for reporting this issue)
* :jirabug:`K8SPXC-842`: Fixed a bug where backup finalizer didn't delete data from S3 if backup path contained a folder inside of the S3 bucket (Thanks to 申祥瑞 for reporting this issue)
* :jirabug:`K8SPXC-812`: Fix a bug due to which the Operator didn't support cert-manager versions since v0.14.0 (Thanks to Ben Langfeld for reporting this issue)
* :jirabug:`K8SPXC-762`: Fix a bug due to which the validating webhook was not accepting scale operation in the Operator cluster-wide mode (Thanks to Henno Schooljan for reporting this issue)
* :jirabug:`K8SPXC-893`: Fix a bug where HAProxy service failed during the config validation check if there was a resolution fail with one fo the PXC addresses
* :jirabug:`K8SPXC-871`: Fix a bug which prevented removing Percona a XtraDB Cluster manual backup for PVC storage
* :jirabug:`K8SPXC-851`: Fixed a bug where changing replication user password didn't work
* :jirabug:`K8SPXC-850`: Fixed a bug where the default weight value wasn't set for a host in a replication channel
* :jirabug:`K8SPXC-845`: Fixed a bug where using malformed cr.yaml caused stuck cases in cluster deletion
* :jirabug:`K8SPXC-838`: Fixed a bug due to which the Log Collector and PMM containers with unspecified memory and CPU requests were inheriting them from the PXC container
* :jirabug:`K8SPXC-824`: Cluster may get into an unrecoverable state with incomplete full crash
* :jirabug:`K8SPXC-818`: Fixed a bug which made Pods with a custom config inside a Secret or a ConfigMap not restarting at config update
* :jirabug:`K8SPXC-783`: Fixed a bug where root user was able bto to modify the monitor and clustercheck system users, makeing the possibility of the cluster failure or misbehavior
* :jirabug:`K8SPXC-822`: LET'S MAKE IT AN IMPROVEMENT? Logrotate now doesn't rotate GRA logs (binlog events in ROW format representing the failed transaction) as ordinary log files, storing them for 7 days instead which gives additional time to debug the problem

