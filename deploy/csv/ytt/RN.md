### Release Highlights

* Starting from this release, the Operator changes its official name to Percona Distribution for MySQL Operator.
  This new name emphasizes gradual changes which incorporated a collection of Percona’s solutions to run and operate Percona Server for MySQL and Percona XtraDB Cluster, available separately as Percona Distribution for MySQL.
  Now you can see HAProxy metrics in your favorite Percona Monitoring and Management (PMM) dashboards automatically.
* The cross-site replication feature allows an asynchronous replication between two Percona XtraDB Clusters, including scenarios when one of the clusters is outside of the Kubernetes environment.
  The feature is intended for the following use cases:
  * provide migrations of your Percona XtraDB Cluster to Kubernetes or vice versa,
  * migrate regular MySQL database to Percona XtraDB Cluster under the Operator control, or carry on backward migration,
  * enable disaster recovery capability for your cluster deployment.
