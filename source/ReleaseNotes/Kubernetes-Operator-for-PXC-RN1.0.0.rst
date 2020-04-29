.. rn:: 1.0.0

*Percona Kubernetes Operator for Percona XtraDB Cluster* 1.0.0
==============================================================

Percona announces the general availability of *Percona Kubernetes Operator for Percona XtraDB Cluster* 1.0.0 on May 29, 2019. This release is now the current GA release in the 1.0 series. `Install the Kubernetes Operator for Percona XtraDB Cluster by following the instructions <https://www.percona.com/doc/kubernetes-operator-for-pxc/kubernetes.html>`__. Please see the `GA release announcement <https://www.percona.com/blog/2019/05/29/percona-kubernetes-operators/>`__. All of Percona's software is open-source and free.

The Percona Kubernetes Operator for Percona XtraDB Cluster automates the lifecycle and provides a consistent Percona XtraDB Cluster instance. The Operator can be used to create a Percona XtraDB Cluster, or scale an existing Cluster and contains the necessary Kubernetes settings.

The Percona Kubernetes Operators are based on best practices for configuration and setup of the Percona XtraDB Cluster. The Operator provides a consistent way to package, deploy, manage, and perform a backup and a restore for a Kubernetes application. Operators deliver automation advantages in cloud-native applications.

The advantages are the following:
  * Deploy a Percona XtraDB Cluster environment with no single point of failure and environment can span multiple availability zones (AZs).
  * Deployment takes about six minutes with the default configuration.
  * Modify the Percona XtraDB Cluster size parameter to add or remove Percona XtraDB Cluster members
  * Integrate with Percona Monitoring and Management (PMM) to seamlessly monitor your Percona XtraDB Cluster
  * Automate backups or perform on-demand backups as needed with support for performing an automatic restore
  * Supports using Cloud storage with S3-compatible APIs for backups
  * Automate the recovery from failure of a single Percona XtraDB Cluster node
  * TLS is enabled by default for replication and client traffic using Cert-Manager
  * Access private registries to enhance security
  * Supports advanced Kubernetes features such as pod disruption budgets, node selector, constraints, tolerations, priority classes, and affinity/anti-affinity
  * You can use either PersistentVolumeClaims or local storage with hostPath to store your database
  * Customize your MySQL configuration using ConfigMap.


Installation
------------

Installation is performed by following the documentation installation instructions for `Kubernetes <https://www.percona.com/doc/kubernetes-operator-for-pxc/kubernetes.html>`__ and `OpenShift <https://www.percona.com/doc/kubernetes-operator-for-pxc/openshift.html>`__.
