 	.. rn:: 1.3.0

Percona Kubernetes Operator for Percona XtraDB Cluster 1.3.0
============================================================

Percona announces the *Percona Kubernetes Operator for Percona XtraDB Cluster*
1.3.0 release on December 20, 2019. This release is now the current GA release
in the 1.3 series. `Install the Kubernetes Operator for Percona XtraDB Cluster
by following the instructions <https://www.percona.com/doc/kubernetes-operator-for-pxc/kubernetes.html>`_.

The Percona Kubernetes Operator for Percona XtraDB Cluster automates the
lifecycle and provides a consistent Percona XtraDB Cluster instance. The
Operator can be used to create a Percona XtraDB Cluster, or scale an existing
Cluster and contains the necessary Kubernetes settings.

The Operator simplifies the deployment and management of the `Percona XtraDB
Cluster <https://www.percona.com/software/mysql-database/percona-xtradb-cluster>`_
in Kubernetes-based environments. It extends the Kubernetes API with a new
custom resource for deploying, configuring and managing the application through
the whole life cycle.

The Operator source code is available `in our Github repository <https://github.com/percona/percona-xtradb-cluster-operator>`_.
All of Percona’s software is open-source and free.

**New features and improvements:**

* :cloudjira:`412`: Auto-Tuning of the MySQL Parameters based on Pod memory
  resources was implemented in case of Percona XtraDB Cluster Pod limits
  (or at least Pod requests) specified in the cr.yaml file.
* :cloudjira:`411`: Now user is able to adjust securityContext, thus replacing
  the automatically generated securityContext with the customized one.
* :cloudjira:`394`: The Percona XtraDB Cluster, ProxySQL, and backup images have
  undergone a 40-60% size decrease due to removing unnecessary dependencies and
  modules to reduce the cluster deployment time.
* :cloudjira:`390`: Helm chart for Percona Monitoring and Management (PMM) 2.0
  have been provided.
* :cloudjira:`383`: Affinity constraints and tolerations were added to the
  backup Pod

**Fixed bugs:**

* :cloudbug:`462`: Resource requests/limits were set not for all containers
  in a ProxySQL Pod
* :cloudbug:`437`: Percona Monitoring and Management Client was taking
  resources definition from the Percona XtraDB Cluster, despite the much lower
  need in resources, particularly lower memory footprint.
* :cloudbug:`434`: Restoring Percona XtraDB Cluster was failing on the
  OpenShift platform
* :cloudbug:`399`: The iputils package was added to the backup docker image to
  provide backup jobs with the ping command
* :cloudbug:`393`: The Operator generated various StatefulSets in the first
  reconciliation cycle and in all subsequent reconciliation cycles, causing
  Kubernetes to trigger an unnecessary ProxySQL restart once during the cluster
  creation.
* :cloudbug:`376`: Long-running SST caused liveness probe check to fail it's
  grace period timeout, resulting in an unrecoverable failure
* :cloudbug:`243`: Using `MYSQL_ROOT_PASSWORD` with special characters in
  proxysql docker image was breaking the the entrypoint initialization process

`Percona XtraDB Cluster <http://www.percona.com/doc/percona-xtradb-cluster/>`_
is an open source, cost-effective and robust clustering solution for businesses.
It integrates Percona Server for MySQL with the Galera replication library to
produce a highly-available and scalable MySQL® cluster complete with synchronous
multi-master replication, zero data loss and automatic node provisioning using
Percona XtraBackup.

Help us improve our software quality by reporting any bugs you encounter using
`our bug tracking system <https://jira.percona.com/secure/Dashboard.jspa>`_.
