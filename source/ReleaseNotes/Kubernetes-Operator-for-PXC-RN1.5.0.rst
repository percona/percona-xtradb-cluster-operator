.. _K8SPXC-1.5.0:

================================================================================
*Percona Kubernetes Operator for Percona XtraDB Cluster* 1.5.0
================================================================================

:Date: July 13, 2020
:Installation: `Installing Percona Kubernetes Operator for Percona XtraDB Cluster <https://www.percona.com/doc/kubernetes-operator-for-psmongodb/index.html#installation>`_

New Features
================================================================================

* :jirabug:`K8SPXC-298`: ProxySQL users should sync automatically
* :jirabug:`K8SPXC-294`: HA Proxy Support
* :jirabug:`K8SPXC-284`: Fully Automated Minor Version Updates
* :jirabug:`K8SPXC-257`: update Secondaries first, before Primary
* :jirabug:`K8SPXC-256`: Support for Multiple Minor Versions in Operators

Improvements
================================================================================

* :jirabug:`K8SPXC-290`: Extend usable backup schedule syntax
* :jirabug:`K8SPXC-332`: improve scaling command
* :jirabug:`K8SPXC-309`: Create Quickstart Guide on Google Kubernetes Engine (GKE)
* :jirabug:`K8SPXC-300`: users doc should mention maximal length of password
* :jirabug:`K8SPXC-288`: Amazon EKS quickstart guide
* :jirabug:`K8SPXC-279`: Use SYSTEM_USER privilege for system users on PXC 8.0
* :jirabug:`K8SPXC-276`: Pod-0 should have priority as writer in ProxySQL
* :jirabug:`K8SPXC-252`: Automatically Manage System Users for MySQL and ProxySQL with Rotation
* :jirabug:`CLOUD-535`: Standard reporting protocol for debugging and troubleshooting
* :jirabug:`CLOUD-556`: Add Kubernetes 1.17 to our support platforms
* :jirabug:`CLOUD-404`: Support of loadBalancerSourceRanges for LoadBalancer Services

Bugs Fixed
================================================================================

* :jirabug:`K8SPXC-327`: SST fail when PXC Pod restarted in the middle of SST
* :jirabug:`K8SPXC-270`: Restore job wiping data from the original backup's cluster when restoring to another cluster in the same namespace
** :jirabug:`K8SPXC-352`: Backup cronjob is not scheduled (Thanks to user msavchenko for reporting this issue)
Backup cronjob is not scheduled
* :jirabug:`K8SPXC-275`: Outdated documentation on the Operator updates (Thanks to user martin.atroo for reporting this issue)
* :jirabug:`K8SPXC-347`: XtraBackup fail after uploading a backup, causing the backup process restart (Thanks to user connde for reporting this issue)
** :jirabug:`K8SPXC-360`: SmartUpdate is not pulling correct haproxy image
** :jirabug:`K8SPXC-357`: failed to check version: received bad status code 404 Not Found
** :jirabug:`K8SPXC-354`: haproxy-replicas service should listen on port 3306
** :jirabug:`K8SPXC-353`: switching from haproxy to proxysql leaves dead services
** :jirabug:`K8SPXC-338`: fix operator logs on smart update finish
* :jirabug:`K8SPXC-331`: pxc-entrypoint.sh: no such file or directory error when running 5.7
* :jirabug:`K8SPXC-330`: missing online nodes in reader hostgroup while upgrade in progress
* :jirabug:`K8SPXC-326`: Research the reason for recreated pod during PXC pod downsizing.
**** :jirabug:`K8SPXC-320`: Broken link to Operators Options Section
**** :jirabug:`K8SPXC-283`: Our backup documentation should mention that storage size is to be configured in the main CR
* :jirabug:`K8SPXC-277`: Install GDB RPM for all PXC images
* :jirabug:`K8SPXC-242`: Backup script will run indefinitely on SST startup error
* :jirabug:`K8SPXC-230`: Backup fail if just one PXC instance running
* :jirabug:`K8SPXC-323`: Missing ``tar`` utility in PXC node docker image
* :jirabug:`K8SPXC-358`: SmartUpdate doesn't recognize lowercase disabled value
* :jirabug:`CLOUD-474`: Cluster creation not failing if wrong resources are set
* :jirabug:`CLOUD-531`: Fix wrong usage of strings.TrimLeft
* :jirabug:`PXC-2987`: SST incompatible between 5.7 and 8.0
* :jirabug:`K8SPXC-310`: Restore from backup likely is broken

