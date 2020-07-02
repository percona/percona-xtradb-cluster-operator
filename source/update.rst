Update Percona XtraDB Cluster Operator
======================================

Starting from the version 1.1.0 the Percona Kubernetes Operator for Percona XtraDB
Cluster allows upgrades to newer versions. The upgrade can be done in three
ways: automatic, semi-automatic, or manual.

.. note:: The manual update mode is the recomended way for a production cluster.

.. note:: Only the incremental update to a nearest minor version is supported
   (for example, update from 1.2.0 to 1.3.0).
   To update to a newer version, which differs from the current version by more
   than one, make several incremental updates sequentially.

Automatic update
----------------

Starting from the version 1.5.0 the Percona Kubernetes Operator for Percona XtraDB
Cluster is able to do fully automatic upgrades to newer versions.

When automatic updates are enabled, the will query the specified
*Version Service* at the specified scheduled time to obtain a set of valid image
paths for the version needed for upgrade. If the current version is different,
the Operator updates the CR to reflect the new image paths and carries on 
sequential Pods deletion in a safe order, allowing StatefulSet to redeploy the
cluster Pods with the new image.

The update is controlled by options in the ``upgradeOptions`` section of the 
``deploy/cr.yaml`` configuration file. Make the following edits to configure
updates:

#. Set ``apply`` option to one of the following values:

   * ``Never`` or ``Disabled`` - automatic upgrades are disabled,
   * ``Recommended`` - automatic upgrades will choose the most recent version,
       of software flagged as Recommended,
   * ``Latest`` - automatic upgrades will choose the most recent version of
     software available,
   * *specific version number* - will apply an upgrade if the running version
     doesn't match the explicit version number with no future upgrades.

#. Make sure the ``versionServiceEndpoint`` key is set to a valid Version
   Server URL. By default it is ``https://check.percona.com/operator/``,
   but you can use your own Version Server as well. Version Server supplies
   the operator with the up-to-date information about versions and their
   compatibility in JSON format.

   .. note:: Version Server is never checked if automatic updates are disabled.

#. Use ``schedule`` option to specify the update checks time in CRON format.

The following example sets the midnight update checks with the official
Percona's Version Service:

.. code:: bash

   spec:
     updateStrategy: SmartUpdate
     upgradeOptions:
       apply: 5.7.27-31.39
       versionServiceEndpoint: https://check.percona.com/operator/
       schedule: "0 0 * * *"
   ...

Semi-automatic update
---------------------

#. Update the Custom Resource Definition file for the Operator, taking it from
   the official repository on Github:

   .. code:: bash

      kubectl apply -f https://raw.githubusercontent.com/percona/percona-xtradb-cluster-operator/release-{{{release}}}/deploy/crd.yaml

#. Edit the ``deploy/cr.yaml`` file, setting ``updateStrategy`` key to
   ``RollingUpdate``.

#. Now you should `apply a patch <https://kubernetes.io/docs/tasks/run-application/update-api-object-kubectl-patch/>`_ to your
   deployment, supplying necessary image names with a newer version tag. This
   is done with the ``kubectl patch deployment`` command. For example, updating
   to the ``{{{release}}}`` version should look as follows, depending on whether
   you are using Percona XtraDB Cluster 5.7 or 8.0.

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

#. Update the Custom Resource Definition file for the Operator, taking it from
   the official repository on Github:

   .. code:: bash

      kubectl apply -f https://raw.githubusercontent.com/percona/percona-xtradb-cluster-operator/release-{{{release}}}/deploy/crd.yaml

#. Edit the ``deploy/cr.yaml`` file, setting ``updateStrategy`` key to
   ``OnDelete``.

#. Now you should `apply a patch <https://kubernetes.io/docs/tasks/run-application/update-api-object-kubectl-patch/>`_ to your
   deployment, supplying necessary image names with a newer version tag. This
   is done with the ``kubectl patch deployment`` command. For example, updating
   to the ``{{{release}}}`` version should look as follows, depending on whether
   you are using Percona XtraDB Cluster 5.7 or 8.0.

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
