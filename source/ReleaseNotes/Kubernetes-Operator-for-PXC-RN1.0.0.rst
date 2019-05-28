.. rn:: 1.0.0

Percona Kubernetes Operator for XtraDB Cluster
==============================================

Percona announces the general availability of |Percona Kubernetes Operator for XtraDB Cluster| 1.0.0 on May 24, 2019. This release is now the current GA release in the 1.0 series. Download the latest version from the Percona Software Repositories. Please see the `GA release announcement`. All of Percona's software is open-source and free.

The Percona Kubernetes Operator for Percona XtraDB Cluster automates the creation, modification, and deletion of members of your Percona XtraDB Cluster environment. The Operator can be used to instantiate a Percona XtraDB Cluster, or scale an existing Cluster.

The Operator contains the necessary Kubernetes settings and provides a consistent Percona XtraDB Cluster instance. The Percona Kubernetes Operators are based on best practices for configuration and setup of the Percona XtraDB Cluster.

The Kubernetes Operators provide a consistent way to package, deploy, manage, and perform a backup and a restore for a Kubernetes application. Operators deliver automation advantages in cloud-native applications and may save time while providing a consistent environment.

The advantages are the following:
  * Deploy a Percona XtraDB Cluster environment with no single point of failure and environment can span multiple zones
  * Modify the Percona XtraDB Cluster size parameter to add or remove Percona XtraDB Cluster members
  * Integrate with Percona Monitoring and Management (PMM) to seamlessly monitor your Percona XtraDB Cluster
  * Automate backups or perform on-demand backups as needed with support for performing a restore
  * Automate the recovery from failure of a single Percona XtraDB Cluster node
  * Provide data encryption in the cluster and between the application and the nodes
  * Access private registries to enhance security


Installation
------------

Installation is performed by accessing the `Percona Software Repositories <https://www.percona.com/doc/kubernetes-operator-for-pxc/kubernetes.html>`__ for Kubernetes and `OpenShift <https://www.percona.com/doc/kubernetes-operator-for-pxc/openshift.html>`__.

Notable Features
--------------------------

ProxySQL 2.0 Support

  * HA ProxySQL Instances using Native Clustering

HA ProxySQL Instances using Native Clustering

Customizable my.cnf

Simplified User Interface for Users to Interact with the Operator

 * :jirabug:`CLOUD-182 <https://jira.percona.com/browse/CLOUD-182>`__ Automatic backup and restore provides a method of performing a hot backup of your MySQL data while the system is running. `Percona XtraBackup <https://www.percona.com/software/mysql-database/percona-xtrabackup>`__ is a free, online, open-source, backup tool.

 * :jirabug:`CLOUD-212 <https://jira.percona.com/browse/CLOUD-212>`__ Uses the `xbcloud <https://www.percona.com/doc/percona-xtrabackup/2.3/xbcloud/xbcloud.html>`__ is part of Percona XtraDB Backup. The purpose of the xbcloud tool is to download and upload full or part of a `xbstream <https://www.percona.com/doc/percona-xtrabackup/2.3/

 * :jirabug:`CLOUD-181 <https://jira.percona.com/browse/CLOUD-181>`__ Kubernetes expects communication within the cluster to be encrypted by default with TLS.

 * :jirabug:`CLOUD-203 < https://jira.percona.com/browse/CLOUD-203>`__ Allows a `manual certificate signing request <https://kubernetes.io/docs/tasks/tls/managing-tls-in-a-cluster/#create-a-certificate-signing-request-object-to-send-to-the-kubernetes-api>`__ to send to the Kubernetes API.

 * :jirabug:`CLOUD-193 <https://jira.percona.com/browse/CLOUD-193>`__ The ProxySQL service is configurable by changing available settings.
