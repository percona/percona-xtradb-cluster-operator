.. rn:: 1.1.0

*Percona Kubernetes Operator for Percona XtraDB Cluster* 1.1.0
==============================================================

Percona announces the general availability of *Percona Kubernetes Operator for Percona XtraDB Cluster* 1.1.0 on July 15, 2019. This release is now the current GA release in the 1.1 series. `Install the Kubernetes Operator for Percona XtraDB Cluster by following the instructions <https://www.percona.com/doc/kubernetes-operator-for-pxc/kubernetes.html>`__.

The Percona Kubernetes Operator for Percona XtraDB Cluster automates the lifecycle and provides a consistent Percona XtraDB Cluster instance. The Operator can be used to create a Percona XtraDB Cluster, or scale an existing Cluster and contains the necessary Kubernetes settings.

The Operator simplifies the deployment and management of the `Percona XtraDB Cluster <https://www.percona.com/software/mysql-database/percona-xtradb-cluster>`_ in Kubernetes-based environments. It extends the Kubernetes API with a new custom resource for deploying, configuring and managing the application through the whole life cycle.

The Operator source code is available `in our Github repository <https://github.com/percona/percona-xtradb-cluster-operator>`_. All of Percona’s software is open-source and free.

**New features and improvements**:

* Now the Percona Kubernetes Operator `allows upgrading <https://www.percona.com/doc/kubernetes-operator-for-pxc/update.html>`_ Percona XtraDB Cluster to newer versions, either in semi-automatic or in manual mode.
* Also, two modes are implemented for updating the Percona XtraDB Cluster ``my.cnf`` configuration file: in *automatic configuration update* mode Percona XtraDB Cluster Pods are immediately re-created to populate changed options from the Operator YAML file, while in *manual mode* changes are held until Percona XtraDB Cluster Pods are re-created manually.
* A separate service account is now used by the Operator's containers which need special privileges, and all other Pods run on default service account with limited permissions.
* `User secrets <https://www.percona.com/doc/kubernetes-operator-for-pxc/users.html>`_ are now generated automatically if don't exist: this feature especially helps reduce work in repeated development environment testing and reduces the chance of accidentally pushing predefined development passwords to production environments.
* The Operator `is now able to generate TLS certificates itself <https://www.percona.com/doc/kubernetes-operator-for-pxc/TLS.html>`_ which removes the need in manual certificate generation.
* The list of officially supported platforms now includes `Minikube <https://www.percona.com/doc/kubernetes-operator-for-pxc/minikube.html>`_, which provides an easy way to test the Operator locally on your own machine before deploying it on a cloud.
* Also, Google Kubernetes Engine 1.14 and OpenShift Platform 4.1 are now supported.

`Percona XtraDB Cluster <http://www.percona.com/doc/percona-xtradb-cluster/>`_ is an open source, cost-effective and robust clustering solution for businesses. It integrates Percona Server for MySQL with the Galera replication library to produce a highly-available and scalable MySQL® cluster complete with synchronous multi-master replication, zero data loss and automatic node provisioning using Percona XtraBackup.

Help us improve our software quality by reporting any bugs you encounter using `our bug tracking system <https://jira.percona.com/secure/Dashboard.jspa>`_.
