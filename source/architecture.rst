Design overview
===============

*Percona XtraDB Cluster* integrates *Percona Server for MySQL* running
with the XtraDB storage engine, and *Percona XtraBackup* with the
*Galera library* to enable synchronous multi-master replication.

The design of the operator is highly bound to the Percona XtraDB Cluster
high availability implementation, which in its turn can be briefly
described with the following diagram.

|PXC HA|\ {: .align-center}

Being a regular MySQL Server instance, each node contains the same set
of data synchronized accross nodes. The recommended configuration is to
have at least 3 nodes. In a basic setup with this amount of nodes,
Percona XtraDB Cluster provides high availability, continuing to
function if you take any of the nodes down. Additionally load balancing
can be achieved with the ProxySQL daemon, which accepts incoming traffic
from MySQL clients and forwards it to backend MySQL servers.

**Note:** *Using ProxySQL results in*\ `more efficient database workload
management <https://proxysql.com/compare>`__\ *in comparison with other
load balancers which are not SQL-aware, including built-in ones of the
cloud providers, or the Kubernetes NGINX Ingress Controller.*

To provide high availability operator uses `node
affinity <https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#affinity-and-anti-affinity>`__
to run PXC instances on separate worker nodes if possible. If some node
fails, the pod with it is automatically re-created on another node.

|The Operator|\ {: .align-center}

To provide data storage for stateful applications, Kubernetes uses
Persistent Volumes. A *PersistentVolumeClaim* (PVC) is used to implement
the automatic storage provisioning to pods. If a failure occurs, the
Container Storage Interface (CSI) should be able to re-mount storage on
a different node. The PVC StorageClass must support this feature
(Kubernetes and OpenShift support this in versions 1.9 and 3.9
respectively).

The Operator functionality extends the Kubernetes API with
*PerconaXtraDBCluster* object, and it is implemented as a golang
application. Each *PerconaXtraDBCluster* object maps to one separate PXC
setup. The Operator listens to all events on the created objects. When a
new PerconaXtraDBCluster object is created, or an existing one undergoes
some changes or deletion, the operator automatically
creates/changes/deletes all needed Kubernetes objects with the
appropriate settings to provide a properly PXC operating.

.. |PXC HA| image:: ./assets/images/replication.png
.. |The Operator| image:: ./assets/images/operator.png
