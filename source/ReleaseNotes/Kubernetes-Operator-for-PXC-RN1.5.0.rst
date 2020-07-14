.. _K8SPXC-1.5.0:

================================================================================
*Percona Kubernetes Operator for Percona XtraDB Cluster* 1.5.0
================================================================================

:Date: July 21, 2020
:Installation: `Installing Percona Kubernetes Operator for Percona XtraDB Cluster <https://www.percona.com/doc/kubernetes-operator-for-psmongodb/index.html#installation>`_

New Features
================================================================================

* :jirabug:`K8SPXC-298`: Automatic synchronization users from MySQL into ProxySQL
* :jirabug:`K8SPXC-294`: HAProxy Support
* :jirabug:`K8SPXC-284`: Fully automated minor version updates
* :jirabug:`K8SPXC-257`: Update Reader members before Writer member when upgrading cluster
* :jirabug:`K8SPXC-256`: Support multiple PXC minor versions by the Operator

Improvements
================================================================================

* :jirabug:`K8SPXC-290`: Extend usable backup schedule syntax to include lists of values
* :jirabug:`K8SPXC-309`: Quickstart Guide on Google Kubernetes Engine (GKE)
* :jirabug:`K8SPXC-288`: Quickstart Guide on Amazon Elastic Kubernetes Service (EKS)
* :jirabug:`K8SPXC-279`: Use SYSTEM_USER privilege for system users on PXC 8.0
* :jirabug:`K8SPXC-277`: Install GDB in all PXC images
* :jirabug:`K8SPXC-276`: Pod-0 should be selected as Writer if possible
* :jirabug:`K8SPXC-252`: Automatically manage system users for MySQL and ProxySQL on password rotation (via secret modification)
* :jirabug:`CLOUD-535`: Standard reporting protocol for debugging and troubleshooting
* :jirabug:`CLOUD-556`: Kubernetes 1.17 added to the list of supported platforms
* :jirabug:`CLOUD-404`: Support of loadBalancerSourceRanges for LoadBalancer Services

Bugs Fixed
================================================================================

* :jirabug:`K8SPXC-327`: SST fail when PXC Pod restarted in the middle of SST
* :jirabug:`K8SPXC-270`: Restore job wiping data from the original backup's cluster when restoring to another cluster in the same namespace
* :jirabug:`K8SPXC-352`: Backup cronjob not scheduled (Thanks to user msavchenko for reporting this issue)
Backup cronjob is not scheduled
* :jirabug:`K8SPXC-275`: Outdated documentation on the Operator updates (Thanks to user martin.atroo for reporting this issue)
* :jirabug:`K8SPXC-347`: XtraBackup fail after uploading a backup, causing the backup process restart (Thanks to user connde for reporting this issue)
* :jirabug:`K8SPXC-331`: pxc-entrypoint.sh: no such file or directory error when running 5.7
* :jirabug:`K8SPXC-330`: missing online nodes in reader hostgroup while upgrade in progress
* :jirabug:`K8SPXC-326`: Pod recreated during PXC Pod downsizing
* :jirabug:`K8SPXC-242`: Backup script running indefinitely on SST startup error
* :jirabug:`K8SPXC-230`: Backup fail if just one PXC instance running
* :jirabug:`K8SPXC-323`: Missing ``tar`` utility in the PXC node docker image
* :jirabug:`CLOUD-474`: Cluster creation not failing if wrong resources are set
* :jirabug:`CLOUD-531`: Wrong usage of ``strings.TrimLeft`` when processing apiVersion
* :jirabug:`PXC-2987`: SST incompatible between 5.7 and 8.0
