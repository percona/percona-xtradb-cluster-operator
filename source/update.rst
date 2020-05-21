Update Percona XtraDB Cluster Operator
======================================

Starting from the version 1.1.0 the Percona Kubernetes Operator for Percona XtraDB
Cluster allows upgrades to newer versions. This upgrade can be done either in
semi-automatic or in manual mode. 

.. note:: The manual update mode is the recomended way for a production cluster.

.. note:: Only the incremental update to a nearest minor version is supported
   (for example, update from 1.2.0 to 1.3.0).
   To update to a newer version, which differs from the current version by more
   than one, make several incremental updates sequentially.

Semi-automatic update
---------------------

#. Edit the ``deploy/cr.yaml`` file, setting ``updateStrategy`` key to
   ``RollingUpdate``.

#. Now you should `apply a patch <https://kubernetes.io/docs/tasks/run-application/update-api-object-kubectl-patch/>`_ to your
   deployment, supplying necessary image names with a newer version tag. This
   is done with the ``kubectl patch deployment`` command. For example, updating
   to the ``{{{release}}}`` version should look as follows.

   A. For Percona XtraDB Cluster 5.7 run the following:

      .. code:: bash

         kubectl patch deployment percona-xtradb-cluster-operator \
            -p'{"spec":{"template":{"spec":{"containers":[{"name":"percona-xtradb-cluster-operator","image":"percona/percona-xtradb-cluster-operator:{{{release}}}"}]}}}}'

         kubectl patch pxc cluster1 --type=merge --patch '{
            "metadata": {"annotations":{ "kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"pxc.percona.com/v{{{apiversion}}}\"}" }},
            "spec": {"pxc":{ "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-pxc5.7" },
                "proxysql": { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-proxysql" },
                "backup":   { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-pxc5.7-backup" },
                "pmm":      { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-pmm" }
            }}'

   B. For Percona XtraDB Cluster 8.0 run the following:

      .. code:: bash

         kubectl patch deployment percona-xtradb-cluster-operator \
            -p'{"spec":{"template":{"spec":{"containers":[{"name":"percona-xtradb-cluster-operator","image":"percona/percona-xtradb-cluster-operator:{{{release}}}"}]}}}}'

         kubectl patch pxc cluster1 --type=merge --patch '{
            "metadata": {"annotations":{ "kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"pxc.percona.com/v{{{apiversion}}}\"}" }},
            "spec": {"pxc":{ "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-pxc8.0" },
                "proxysql": { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-proxysql" },
                "backup":   { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-pxc8.0-backup" },
                "pmm":      { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-pmm" }
            }}'

#. The deployment rollout will be automatically triggered by the applied patch.
   You can track the rollout process in real time with the
   ``kubectl rollout status`` command with the name of your cluster::

     kubectl rollout status sts cluster1-pxc

Manual update
-------------

#. Edit the ``deploy/cr.yaml`` file, setting ``updateStrategy`` key to
   ``OnDelete``.

#. Now you should `apply a patch <https://kubernetes.io/docs/tasks/run-application/update-api-object-kubectl-patch/>`_ to your
   deployment, supplying necessary image names with a newer version tag. This
   is done with the ``kubectl patch deployment`` command. For example, updating
   to the ``{{{release}}}`` version should look as follows.

   A. For Percona XtraDB Cluster 5.7 run the following:

      .. code:: bash

         kubectl patch deployment percona-xtradb-cluster-operator \
            -p'{"spec":{"template":{"spec":{"containers":[{"name":"percona-xtradb-cluster-operator","image":"percona/percona-xtradb-cluster-operator:{{{release}}}"}]}}}}'

         kubectl patch pxc cluster1 --type=merge --patch '{
            "metadata": {"annotations":{ "kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"pxc.percona.com/v{{{apiversion}}}\"}" }},
            "spec": {"pxc":{ "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-pxc5.7" },
                "proxysql": { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-proxysql" },
                "backup":   { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-pxc5.7-backup" },
                "pmm":      { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-pmm" }
            }}'

   B. For Percona XtraDB Cluster 8.0 run the following:

      .. code:: bash

         kubectl patch deployment percona-xtradb-cluster-operator \
            -p'{"spec":{"template":{"spec":{"containers":[{"name":"percona-xtradb-cluster-operator","image":"percona/percona-xtradb-cluster-operator:{{{release}}}"}]}}}}'

         kubectl patch pxc cluster1 --type=merge --patch '{
            "metadata": {"annotations":{ "kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"pxc.percona.com/v{{{apiversion}}}\"}" }},
            "spec": {"pxc":{ "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-pxc8.0" },
                "proxysql": { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-proxysql" },
                "backup":   { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-pxc8.0-backup" },
                "pmm":      { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-pmm" }
            }}'

#. The Pod with the newer Percona XtraDB Cluster image will start after you
   delete it. Delete targeted Pods manually one by one to make them restart in
   desired order:

   #. Delete the Pod using its name with the command like the following one::

         kubectl delete pod cluster1-pxc-2

   #. Wait until Pod becomes ready::

         kubectl get pod cluster1-pxc-2

      The output should be like this::

         NAME             READY   STATUS    RESTARTS   AGE
         cluster1-pxc-2   1/1     Running   0          3m33s

#. The update process is successfully finished when all Pods have been
   restarted.
