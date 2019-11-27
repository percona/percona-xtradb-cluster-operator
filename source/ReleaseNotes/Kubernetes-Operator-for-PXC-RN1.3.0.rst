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

* :cloudbug:`CLOUD-448`: [DBaaS] Create a standardized go-package to interact with Operators
* :cloudbug:`433`: Add git tags for Xtradb cluster backup image github repo to
  match the Docker tag
* :cloudbug:`430`: Update image URL in the CronJob Pod template  when backup
  image URL changes in the cr.yaml file.
* :cloudbug:`412`: Auto-Tuning MySQL Parameters based on Pod resources (memory
  and CPU) was inplemented in case of PXC Pod limits or at least PXC Pod
  requests specified in the cr.yaml file.
* :cloudbug:`411`: Now user is able to to adjust securityContext, thus replacing
  the automatically generated securityContext with the customized one.
* :cloudbug:`394`: Decrease PXC, ProxySQL, and backup images size by half,
  removing non-necessary dependencies and modules.
* :cloudbug:`390`: Helm chart for PMM 2.0 have been provided

**Fixed bugs:**

* :cloudbug:`466`: Fix issues with affinity and toleration for backup pods
* :cloudbug:`462`: Resource requests/limits were set not for all containers
* :cloudbug:`437`: PMM Client was taking the resources definition from the PXC,
  despite much lower need in resources, particularly lower memory footprint.
* :cloudbug:`434`: Restoring Percona Xtradb Cluster was failing on the
  OpenShift platform
* :cloudbug:`399`: iputils package was added to the backub docker image to
  provide jbackub jobs with the ping command
* :cloudbug:`393`: Fix StatefulSet generation bump for ProxySQL if PMM enabled
* :cloudbug:`383`: Affinity constraints and tolerations were added to the backup
  container
* :cloudbug:`376`: Long-running SST caused liveness probe check to fail  it's grace
  period timeout, resulting in an unrecoverable failure
* :cloudbug:`243`: Using `MYSQL_ROOT_PASSWORD` with special characters in
  proxysql docker image was breaking the the entrypoint initialisation process

`Percona XtraDB Cluster <http://www.percona.com/doc/percona-xtradb-cluster/>`_
is an open source, cost-effective and robust clustering solution for businesses.
It integrates Percona Server for MySQL with the Galera replication library to
produce a highly-available and scalable MySQL® cluster complete with synchronous
multi-master replication, zero data loss and automatic node provisioning using
Percona XtraBackup.

Help us improve our software quality by reporting any bugs you encounter using
`our bug tracking system <https://jira.percona.com/secure/Dashboard.jspa>`_.
