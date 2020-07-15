.. _operator-update:

Upgrade Percona XtraDB Cluster
======================================

Starting from version 1.1.0, Percona Kubernetes Operator for Percona
XtraDB Cluster allows upgrades to newer versions. This includes upgrades of the
Operator itself, and upgrades of the Percona XtraDB Cluster.

.. contents:: :local:

.. _operator-update:

Upgrading the Operator
----------------------

This upgrade can be done either in semi-automatic or in manual mode. 

.. note:: The manual update mode is the recomended way for a production cluster.

.. _operator-update-semi-auto-updates:

Semi-automatic upgrade
**********************

.. note:: Only the incremental update to a nearest minor version of the Operator
   is supported (for example, update from 1.2.0 to 1.3.0).
   To update to a newer version, which differs from the current version by more
   than one, make several incremental updates sequentially.

#. Update the Custom Resource Definition file for the Operator, taking it from
   the official repository on Github, and do the same for the Role-based access
   control:

   .. code:: bash

      kubectl apply -f https://raw.githubusercontent.com/percona/percona-xtradb-cluster-operator/release-{{{release}}}/deploy/crd.yaml
      kubectl apply -f https://raw.githubusercontent.com/percona/percona-xtradb-cluster-operator/release-{{{release}}}/deploy/rbac.yaml

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

.. _operator-update-manual-updates:

Manual update
*************

.. note:: Only the incremental update to a nearest minor version of the Operator
   is supported (for example, update from 1.2.0 to 1.3.0).
   To update to a newer version, which differs from the current version by more
   than one, make several incremental updates sequentially.

#. Update the Custom Resource Definition file for the Operator, taking it from
   the official repository on Github, and do the same for the Role-based access
   control:

   .. code:: bash

      kubectl apply -f https://raw.githubusercontent.com/percona/percona-xtradb-cluster-operator/release-{{{release}}}/deploy/crd.yaml
      kubectl apply -f https://raw.githubusercontent.com/percona/percona-xtradb-cluster-operator/release-{{{release}}}/deploy/rbac.yaml

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

.. _operator-update-smartupdates:

Upgrading Percona XtraDB Cluster
--------------------------------

Starting from version 1.5.0, the Operator can do fully automatic upgrades to
the newer versions of Percona XtraDB Cluster within the method named *Smart
Updates*.

To have this upgrade method enabled, make sure that the ``updateStrategy`` key
in the ``deploy/cr.yaml`` configuration file is set to ``SmartUpdate``.

When automatic updates are enabled, the Operator will carry on upgrades
according to the following algorithm. It will query a special *Version Service* 
server at scheduled times to obtain fresh information about version numbers and
valid image paths needed for the upgrade. If the current version should be
upgraded, the Operator updates the CR to reflect the new image paths and carries
on sequential Pods deletion in a safe order, allowing StatefulSet to redeploy
the cluster Pods with the new image.

The upgrade details are set in the ``upgradeOptions`` section of the 
``deploy/cr.yaml`` configuration file. Make the following edits to configure
updates:

#. Set the ``apply`` option to one of the following values:

   * ``Recommended`` - automatic upgrades will choose the most recent version
     of software flagged as Recommended,
   * ``Latest`` - automatic upgrades will choose the most recent version of
     the software available,
   * *specific version number* - will apply an upgrade if the running PXC
     version doesn't match the explicit version number with no future upgrades
     (version numbers are specified as ``5.7.26-31.37``, ``5.7.27-31.39``,
     ``5.7.28-31.41.2``, ``5.7.29-31.43``, etc.),
   * ``Never`` or ``Disabled`` - disable automatic upgrades
     
     .. note:: When automatic upgrades are disabled by the ``apply`` option, 
        Smart Update functionality will continue working for changes triggered
        by other events, such as updating a ConfigMap, rotating a password, or
        changing resource values.

#. Make sure the ``versionServiceEndpoint`` key is set to a valid Version
   Server URL (otherwise Smart Updates will not occur).

   A. You can use the URL of the official Percona's Version Service (default).
      Set ``versionServiceEndpoint`` to ``https://check.percona.com/versions``.

   B. Alternatively, you can run Version Service inside your cluster. This
      can be done with the ``kubectl`` command as follows:
      
      .. code:: bash
      
         kubectl run version-service --image=perconalab/version-service --env="SERVE_HTTP=true" --port 11000 --expose

   .. note:: Version Service is never checked if automatic updates are disabled.
      If automatic updates are enabled, but Version Service URL can not be
      reached, upgrades will not occur.

#. Use the ``schedule`` option to specify the update checks time in CRON format.

The following example sets the midnight update checks with the official
Percona's Version Service:

.. code:: yaml

   spec:
     updateStrategy: SmartUpdate
     upgradeOptions:
       apply: Recommended
       versionServiceEndpoint: https://check.percona.com/versions
       schedule: "0 0 * * *"
   ...

.. _operator-update-semiauto-updates:

