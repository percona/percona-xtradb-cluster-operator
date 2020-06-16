.. _K8SPXC-1.5.0:

================================================================================
*Percona Kubernetes Operator for Percona XtraDB Cluster* 1.5.0
================================================================================

:Date: July 13, 2020
:Installation: `Installing Percona Kubernetes Operator for Percona XtraDB Cluster <https://www.percona.com/doc/kubernetes-operator-for-psmongodb/index.html#installation>`_

New Features
================================================================================

* :jirabug:`K8SPXC-256`: Support for Multiple Minor Versions in Operators
* :jirabug:`K8SPXC-294`: HA Proxy Support

Improvements
================================================================================

* :jirabug:`K8SPXC-290`: Extend usable backup schedule syntax
* :jirabug:`K8SPXC-288`: Amazon EKS quickstart guide
* :jirabug:`K8SPXC-279`: Use SYSTEM_USER privilege for system users on PXC 8.0
* :jirabug:`CLOUD-535`: Standard reporting protocol for debugging and troubleshooting
* :jirabug:`CLOUD-404`: Support of loadBalancerSourceRanges for LoadBalancer Services

Bugs Fixed
================================================================================

* :jirabug:`K8SPXC-270`: Restore job wiping data from original backup's cluster when restoring to another cluster in the same namespace
* :jirabug:`K8SPXC-275`: Documentation on Operator Updates incomplete
* :jirabug:`K8SPXC-283`: Documentation not mentioning that storage size is to be configured in the main CR
* :jirabug:`K8SPXC-277`: Install GDB RPM for all PXC images
* :jirabug:`PXC-2987`: SST incompatible between 5.7 and 8.0
* :jirabug:`K8SPXC-253`: Changes on CR are not rolled out
* :jirabug:`K8SPXC-230`: Backup fail if just one PXC instance running
* :jirabug:`CLOUD-531`: Fix wrong usage of strings.TrimLeft
* :jirabug:`PMM-5350`: PMM 500 Internal Server Error due to file permissions
* :jirabug:`K8SPXC-298`: ProxySQL users not propagating automatically
* :jirabug:`CLOUD-536`: Restore from backup broken
* :jirabug:`CLOUD-474`: Cluster creation not failing if wrong resources are set
* :jirabug:`CLOUD-534`: No Failed backup status in case of unsuccessful backup

