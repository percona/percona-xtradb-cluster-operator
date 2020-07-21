.. rn:: 1.2.0

*Percona Kubernetes Operator for Percona XtraDB Cluster* 1.2.0
==============================================================

Percona announces the *Percona Kubernetes Operator for Percona XtraDB Cluster*
1.2.0 release on September 20, 2019. This release is now the current GA release
in the 1.2 series. `Install the Kubernetes Operator for Percona XtraDB Cluster
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

* `A Service Broker was implemented <https://www.percona.com/doc/kubernetes-operator-for-pxc/broker.html>`_
  for the Operator, allowing a user to deploy Percona XtraDB Cluster on the
  OpenShift Platform, configuring it with a standard GUI, following the Open
  Service Broker API.
* Now the Operator supports `Percona Monitoring and Management 2 <https://www.percona.com/doc/percona-monitoring-and-management/2.x/index.html>`_,
  which means being able to detect and register to PMM Server of both 1.x and
  2.0 versions.
* A ``NodeSelector`` constraint is now supported for the backups, which allows
  using backup storage accessible to a limited set of nodes only (contributed
  by `Chen Min <https://github.com/chenmin1992>`_).
* The resource constraint values were refined for all containers to eliminate
  the possibility of an out of memory error.
* Now it is possible to set the ``schedulerName`` option in the operator
  parameters. This allows using storage which depends on a custom scheduler, or
  a cloud provider which optimizes scheduling to run workloads in a
  cost-effective way (contributed by `Smaine Kahlouch <https://github.com/Smana>`_).
* A bug was fixed, which made cluster status oscillate between "initializing"
  and "ready" after an update.
* A 90 second startup delay which took place on freshly deployed Percona XtraDB
  Cluster was eliminated.

`Percona XtraDB Cluster <http://www.percona.com/doc/percona-xtradb-cluster/>`_
is an open source, cost-effective and robust clustering solution for businesses.
It integrates Percona Server for MySQL with the Galera replication library to
produce a highly-available and scalable MySQL® cluster complete with synchronous
multi-primary replication, zero data loss and automatic node provisioning using
Percona XtraBackup.

Help us improve our software quality by reporting any bugs you encounter using
`our bug tracking system <https://jira.percona.com/secure/Dashboard.jspa>`_.
