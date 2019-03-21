Requirements and limitations
-------------------------------------------

The operator was developed and/or tested for the following configurations only:

1. Percona XtraDB Cluster 5.7

2. OpenShift 3.9 and OpenShift 4.0

Other options may or may not work.

Also, current PXC on Kubernetes implementation is subject to the following restrictions:

1. Only one instance of ProxySQL is used for load balancing.

2. Backups are not yet supported.
