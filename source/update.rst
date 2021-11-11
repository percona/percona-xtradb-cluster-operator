.. _operator-upgrade:

Update Percona Distribution for MySQL Operator
==============================================

Starting from version 1.1.0, Percona Distribution for MySQL Operator
allows upgrades to newer versions. This includes upgrades of the
Operator itself, and upgrades of the Percona XtraDB Cluster.

.. contents:: :local:

.. _operator-update:

Upgrading the Operator
----------------------

The Operator upgrade includes the following steps.

#. Update the Custom Resource Definition file for the Operator, taking it from
   the official repository on Github, and do the same for the Role-based access
   control:

   .. code:: bash

      $ kubectl apply -f https://raw.githubusercontent.com/percona/percona-xtradb-cluster-operator/v{{{release}}}/deploy/crd.yaml
      $ kubectl apply -f https://raw.githubusercontent.com/percona/percona-xtradb-cluster-operator/v{{{release}}}/deploy/rbac.yaml

#. Now you should `apply a patch <https://kubernetes.io/docs/tasks/run-application/update-api-object-kubectl-patch/>`_ to your
   deployment, supplying necessary image name with a newer version tag. This
   is done with the ``kubectl patch deployment`` command. You can found proper
   image name :ref:`in the list of certified images<custom-registry-images>`.
   For example, updating to the ``{{{release}}}`` version should look as
   follows.

   .. code:: bash

      $ kubectl patch deployment percona-xtradb-cluster-operator \
        -p'{"spec":{"template":{"spec":{"containers":[{"name":"percona-xtradb-cluster-operator","image":"percona/percona-xtradb-cluster-operator:{{{release}}}"}]}}}}'

#. The deployment rollout will be automatically triggered by the applied patch.
   You can track the rollout process in real time with the
   ``kubectl rollout status`` command with the name of your cluster::

     $ kubectl rollout status deployments percona-xtradb-cluster-operator

.. note:: Labels set on the Operator Pod will not be updated during upgrade.

.. _operator-update-smartupdates:

Upgrading Percona XtraDB Cluster
--------------------------------

Automatic upgrade
*****************

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
     of software flagged as Recommended (for clusters created from scratch,
     the Percona XtraDB Cluster 8.0 version will be selected instead of the
     Percona XtraDB Cluster 5.7 one regardless of the image path; for already
     existing clusters, the 8.0 vs. 5.7 branch choice will be preserved),
   * ``8.0-recommended``, ``5.7-recommended`` - same as above, but preserves
     specific major Percona XtraDB Cluster version for newly provisioned
     clusters (ex. 8.0 will not be automatically used instead of 5.7),
   * ``Latest`` - automatic upgrades will choose the most recent version of
     the software available,
   * ``8.0-latest``, ``5.7-latest`` - same as above, but preserves specific
     major Percona XtraDB Cluster version for newly provisioned
     clusters (ex. 8.0 will not be automatically used instead of 5.7),
   * *version number* - specify the desired version explicitly
     (version numbers are specified as ``{{{pxc80recommended}}}``,
     ``{{{pxc57recommended}}}``, etc.),
   * ``Never`` or ``Disabled`` - disable automatic upgrades

     .. note:: When automatic upgrades are disabled by the ``apply`` option, 
        Smart Update functionality will continue working for changes triggered
        by other events, such as updating a ConfigMap, rotating a password, or
        changing resource values.

#. Make sure the ``versionServiceEndpoint`` key is set to a valid Version
   Server URL (otherwise Smart Updates will not occur).

   A. You can use the URL of the official Percona's Version Service (default).
      Set ``versionServiceEndpoint`` to ``https://check.percona.com``.

   B. Alternatively, you can run Version Service inside your cluster. This
      can be done with the ``kubectl`` command as follows:
      
      .. code:: bash
      
         $ kubectl run version-service --image=perconalab/version-service --env="SERVE_HTTP=true" --port 11000 --expose

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
       versionServiceEndpoint: https://check.percona.com
       schedule: "0 0 * * *"
   ...

.. _operator-update-semi-auto-updates:

Semi-automatic upgrade
**********************

Semi-automatic update of Percona XtraDB Cluster should be used with the Operator
version 1.5.0 or earlier. For all newer versions, use :ref:`automatic update<operator-update-smartupdates>`
instead.

#. Edit the ``deploy/cr.yaml`` file, setting ``updateStrategy`` key to
   ``RollingUpdate``.

#. Now you should `apply a patch <https://kubernetes.io/docs/tasks/run-application/update-api-object-kubectl-patch/>`_ to your
   Custom Resource, setting necessary image names with a newer version tag. 
   Also, you should specify the Operator version for your Percona XtraDB Cluster
   as a ``crVersion`` value. This version should be lower or equal to the
   version of the Operator you currently have in your Kubernetes environment.

   .. note:: Only the incremental update to a nearest minor version of the
      Operator is supported (for example, update from 1.4.0 to 1.5.0). To update
      to a newer version, which differs from the current version by more
      than one, make several incremental updates sequentially.

   Patching Custom Resource is done with the ``kubectl patch pxc`` command.
   Actual image names can be found :ref:`in the list of certified images<custom-registry-images>`.
   For example, updating to the ``{{{release}}}`` version should look as
   follows, depending on whether you are using Percona XtraDB Cluster 5.7 or 8.0.

   A. For Percona XtraDB Cluster 5.7 run the following:

      .. code:: bash

         kubectl patch pxc cluster1 --type=merge --patch '{
            "spec": {
                "crVersion":"{{{release}}}",
                "pxc":{ "image": "percona/percona-xtradb-cluster:{{{pxc57recommended}}}" },
                "proxysql": { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-proxysql" },
                "haproxy":  { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-haproxy" },
                "backup":   { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-pxc5.7-backup" },
                "logcollector": { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-logcollector" },
                "pmm":      { "image": "percona/pmm-client:{{{pmm2recommended}}}" }
            }}'

   B. For Percona XtraDB Cluster 8.0 run the following:

      .. code:: bash

         kubectl patch pxc cluster1 --type=merge --patch '{
            "spec": {
                "crVersion":"{{{release}}}",
                "pxc":{ "image": "percona/percona-xtradb-cluster:{{{pxc80recommended}}}" },
                "proxysql": { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-proxysql" },
                "haproxy":  { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-haproxy" },
                "backup":   { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-pxc8.0-backup" },
                "logcollector": { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-logcollector" },
                "pmm":      { "image": "percona/pmm-client:{{{pmm2recommended}}}" }
            }}'

#. The deployment rollout will be automatically triggered by the applied patch.
   You can track the rollout process in real time with the
   ``kubectl rollout status`` command with the name of your cluster::

     $ kubectl rollout status sts cluster1-pxc

.. _operator-update-manual-updates:

Manual upgrade
**************

Manual update of Percona XtraDB Cluster should be used with the Operator
version 1.5.0 or earlier. For all newer versions, use :ref:`automatic update<operator-update-smartupdates>`
instead.

#. Edit the ``deploy/cr.yaml`` file, setting ``updateStrategy`` key to
   ``OnDelete``.

#. Now you should `apply a patch <https://kubernetes.io/docs/tasks/run-application/update-api-object-kubectl-patch/>`_ to your
   Custom Resource, setting necessary image names with a newer version tag. 
   Also, you should specify the Operator version for your Percona XtraDB Cluster
   as a ``crVersion`` value. This version should be lower or equal to the
   version of the Operator you currently have in your Kubernetes environment.

   .. note:: Only the incremental update to a nearest minor version of the
      Operator is supported (for example, update from 1.4.0 to 1.5.0). To update
      to a newer version, which differs from the current version by more
      than one, make several incremental updates sequentially.

   Patching Custom Resource is done with the ``kubectl patch pxc`` command.
   Actual image names can be found :ref:`in the list of certified images<custom-registry-images>`.
   For example, updating to the ``{{{release}}}`` version should look as
   follows, depending on whether you are using Percona XtraDB Cluster 5.7 or 8.0.

   A. For Percona XtraDB Cluster 5.7 run the following:

      .. code:: bash

         $ kubectl patch pxc cluster1 --type=merge --patch '{
             "spec": {
                "crVersion":"{{{release}}}",
                "pxc":{ "image": "percona/percona-xtradb-cluster:{{{pxc57recommended}}}" },
                "proxysql": { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-proxysql" },
                "haproxy":  { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-haproxy" },
                "backup":   { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-pxc5.7-backup" },
                "logcollector": { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-logcollector" },
                "pmm":      { "image": "percona/pmm-client:{{{pmm2recommended}}}" }
             }}'

   B. For Percona XtraDB Cluster 8.0 run the following:

      .. code:: bash


         $ kubectl patch pxc cluster1 --type=merge --patch '{
             "spec": {
                "crVersion":"{{{release}}}",
                "pxc":{ "image": "percona/percona-xtradb-cluster:{{{pxc80recommended}}}" },
                "proxysql": { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-proxysql" },
                "haproxy":  { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-haproxy" },
                "backup":   { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-pxc8.0-backup" },
                "logcollector": { "image": "percona/percona-xtradb-cluster-operator:{{{release}}}-logcollector" },
                "pmm":      { "image": "percona/pmm-client:{{{pmm2recommended}}}" }
             }}'

#. The Pod with the newer Percona XtraDB Cluster image will start after you
   delete it. Delete targeted Pods manually one by one to make them restart in
   desired order:

   #. Delete the Pod using its name with the command like the following one::

        $ kubectl delete pod cluster1-pxc-2

   #. Wait until Pod becomes ready::

        $ kubectl get pod cluster1-pxc-2

      The output should be like this::

         NAME             READY   STATUS    RESTARTS   AGE
         cluster1-pxc-2   1/1     Running   0          3m33s

#. The update process is successfully finished when all Pods have been
   restarted.
