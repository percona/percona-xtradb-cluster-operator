Update Percona XtraDB Cluster Operator
======================================

Starting from the version 1.1.0 Percona Kubernetes Operator for Percona XtraDB
Cluster allows upgrades to newer versions. This upgrade can be done either in
semi-automatic or in manual mode.

.. note:: Manual update mode is the recomended way for a production cluster.

Semi-automatic update
---------------------

#. Edit the ``deploy/cr.yaml`` file, setting ``updateStrategy`` key to
   ``RollingUpdate``.

#. Now you should `apply a patch <https://kubernetes.io/docs/tasks/run-application/update-api-object-kubectl-patch/>`_ to your
   deployment, supplying necessary image names with a newer version tag. This
   is done with the ``kubectl patch deployment`` command. For example, updating
   to ``1.1.0`` version should look as follows::

     kubectl patch deployment percona-xtradb-cluster-operator \
        -p'{"spec":{"template":{"spec":{"containers":[{"name":"percona-xtradb-cluster-operator","image":"percona/percona-xtradb-cluster-operator:1.1.0"}]}}}}'

     kubectl patch pxc cluster1 --type=merge --patch '{
        "spec": {"pxc":{ "image": "percona/percona-xtradb-cluster-operator:1.1.0-pxc" },
            "proxysql": { "image": "percona/percona-xtradb-cluster-operator:1.1.0-proxysql" },
            "backup":   { "image": "percona/percona-xtradb-cluster-operator:1.1.0-backup" }
        }}'

#. The deployment rollout will be automatically triggered by the applied patch.
   You can track the rolluot process in real time with the
   ``kubectl rollout status`` command with the name of your cluster::

     kubectl rollout status sts cluster1-pxc

Manual update
-------------

#. Edit the ``deploy/cr.yaml`` file, setting ``updateStrategy`` key to
   ``OnDelete``.

#. Now you should `apply a patch <https://kubernetes.io/docs/tasks/run-application/update-api-object-kubectl-patch/>`_ to your
   deployment, supplying necessary image names with a newer version tag. This
   is done with the ``kubectl patch deployment`` command. For example, updating
   to ``1.1.0`` version should look as follows::

     kubectl patch deployment percona-xtradb-cluster-operator \
        -p'{"spec":{"template":{"spec":{"containers":[{"name":"percona-xtradb-cluster-operator","image":"percona/percona-xtradb-cluster-operator:1.1.0"}]}}}}'

     kubectl patch pxc cluster1 --type=merge --patch '{
        "spec": {"pxc":{ "image": "percona/percona-xtradb-cluster-operator:1.1.0-pxc" },
            "proxysql": { "image": "percona/percona-xtradb-cluster-operator:1.1.0-proxysql" },
            "backup":   { "image": "percona/percona-xtradb-cluster-operator:1.1.0-backup" }
        }}'

#. Pod with the newer Percona XtraDB Cluster image will start after you
   you delete it. Delete targeted Pods to make them restart manually, one by one:

   #. Delete the Pod using its name with the command like the following one::

         kubectl delete pod cluster1-pxc-2

   #. Wait untill Pod becomes ready::

         kubectl get pod cluster1-pxc-2

      The output should be like this::

         NAME             READY   STATUS    RESTARTS   AGE
         cluster1-pxc-2   1/1     Running   0          3m33s

#. The update process is successfully finished when all Pods have been
   restarted.
