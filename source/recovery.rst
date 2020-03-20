Crash Recovery
=================

What does the full cluster crash mean?
---------------------------------------

A full cluster crash is a situation when all database instances where
shut down in random order. Being rebooted after such situation, Pod is
continuously restarting, and generates the following errors in the log::

  It may not be safe to bootstrap the cluster from this node. It was not the last one to leave the cluster and may not contain all the updates. 
  To force cluster bootstrap with this node, edit the grastate.dat file manually and set safe_to_bootstrap to 1

.. note:: To avoid this, shutdown your cluster correctly
   as it is written in :ref:`operator-pause`.

Obviously, these continuous restarts prevent to get console access to the container,
and so a special approach is needed to make fixes.

The Percona Operator for Percona XtraDB Cluster provides two ways of recovery
after a full cluster crash.

* The automated :ref:`recovery-bootstrap` is the simplest one, but it
  may cause loss of several recent transactions.
* The manual :ref:`recovery-object-surgery` includes a lot of operations, but
  it allows to restore all the data.

.. _recovery-bootstrap:

Bootstrap Crash Recovery method
-------------------------------

In this case recovery is done automatically. The recovery is triggered by the
``pxc.forceUnsafeBootstrap`` option set to ``true`` in the ``deploy/cr.yaml``
file::

     pxc:
       ...
       forceUnsafeBootstrap: true

Applying this option forces the cluster to start. However, there may exist data
inconsistency in the cluster, and several last transactions may be lost. 
If such data loss is undesirable, experienced users may choose the more advanced
manual method described in the next chapter.

.. _recovery-object-surgery:

Object Surgery Crash Recovery method
------------------------------------

.. warning:: This method is intended for advanced users only!

This method involves the following steps:
* swap the original PXC image with the debug image, which does not reboot after
  the crash, and force all Pods to run it,
* find the Pod with the most recent PXC data, run recovery on it, start
  ``mysqld``, and allow the cluster to be restarted,
* revert all temporary substitutions.

Let's assume that a full crash did occur for the cluster named ``cluster1``,
which is based on three PXC Pods.

.. note:: The following commands are written for PXC 8.0. The same steps are
   also for PXC 5.7 unless specifically indicated otherwise.

1. Change the normal PXC image inside the cluster object to the debug image:

   .. code-block:: bash

      $ kubectl patch pxc cluster1 --type="merge" -p '{"spec":{"pxc":{"image":"percona/percona-xtradb-cluster-operator:{{{release}}}-pxc8.0-debug"}}}'

   .. note:: For PXC 5.7 this command should be as follows:

      .. code-block:: bash

         $ kubectl patch pxc cluster1 --type="merge" -p '{"spec":{"pxc":{"image":"percona/percona-xtradb-cluster-operator:{{{release}}}-pxc5.7-debug"}}}'

2.  Restart all Pods:

   .. code-block:: bash

      $ $ for i in $(seq 0 $(($(kubectl get pxc cluster1 -o jsonpath='{.spec.pxc.size}')-1))); do kubectl delete pod cluster1-pxc-$i --force --grace-period=0; done

3. Wait until the Pod ``0`` is ready, and execute the following code (it is
   required for the Pod liveness check):

   .. code-block:: bash

      $ for i in $(seq 0 $(($(kubectl get pxc cluster1 -o jsonpath='{.spec.pxc.size}')-1))); do until [[ $(kubectl get pod cluster1-pxc-$i -o jsonpath='{.status.phase}') == 'Running' ]]; do sleep 10; done; kubectl exec cluster1-pxc-$i -- touch /var/lib/mysql/sst_in_progress; done

4. Wait for all PXC Pods to start, then find the PXC instance with the most
   recent data - i.e. the one with the highest `sequence number (seqno) <https://www.percona.com/blog/2017/12/14/sequence-numbers-seqno-percona-xtradb-cluster/>`_:

   .. code-block:: bash

      $ for i in $(seq 0 $(($(kubectl get pxc cluster1 -o jsonpath='{.spec.pxc.size}')-1))); do echo "###############cluster1-pxc-$i##############"; kubectl exec cluster1-pxc-$i -- cat /var/lib/mysql/grastate.dat; done

   The output of this command should be similar to the following one::

      ###############cluster1-pxc-0##############
      # GALERA saved state
      version: 2.1
      uuid:    7e037079-6517-11ea-a558-8e77af893c93
      seqno:   18
      safe_to_bootstrap: 0
      ###############cluster1-pxc-1##############
      # GALERA saved state
      version: 2.1
      uuid:    7e037079-6517-11ea-a558-8e77af893c93
      seqno:   18
      safe_to_bootstrap: 0
      ###############cluster1-pxc-2##############
      # GALERA saved state
      version: 2.1
      uuid:    7e037079-6517-11ea-a558-8e77af893c93
      seqno:   19
      safe_to_bootstrap: 0

   Now find the Pod with the largest ``seqno`` (it is ``cluster1-pxc-2`` in the
   above example).

5. Now execute the following commands *in a separate shell* to start this
   instance:

   .. code-block:: bash

      $ kubectl exec cluster1-pxc-2 -- mysqld --wsrep_recover
      $ kubectl exec cluster1-pxc-2 -- sed -i 's/safe_to_bootstrap: 0/safe_to_bootstrap: 1/g' /var/lib/mysql/grastate.dat
      $ kubectl exec cluster1-pxc-2 -- sed -i 's/wsrep_cluster_address=.*/wsrep_cluster_address=gcomm:\/\//g' /etc/mysql/node.cnf
      $ kubectl exec cluster1-pxc-2 -- mysqld

   The ``mysqld`` process will initialize the database once again, and it will
   be available for the incoming connections.

6. Go back *to the previous shell* and return the original PXC image because the
   debug image is no longer needed:

   .. code-block:: bash

      $ kubectl patch pxc cluster1 --type="merge" -p '{"spec":{"pxc":{"image":"percona/percona-xtradb-cluster-operator:{{{release}}}-pxc8.0"}}}'

   .. note:: For PXC 5.7 this command should be as follows:

      .. code-block:: bash

         $ kubectl patch pxc cluster1 --type="merge" -p '{"spec":{"pxc":{"image":"percona/percona-xtradb-cluster-operator:{{{release}}}-pxc5.7"}}}'

7. Restart all Pods besides the ``cluster1-pxc-2`` Pod (the recovery donor).

   .. code-block:: bash

      $ for i in $(seq 0 $(($(kubectl get pxc cluster1 -o jsonpath='{.spec.pxc.size}')-1))); do until [[ $(kubectl get pod cluster1-pxc-$i -o jsonpath='{.status.phase}') == 'Running' ]]; do sleep 10; done; kubectl exec cluster1-pxc-$i -- rm /var/lib/mysql/sst_in_progress; done
      $ kubectl delete pods --force --grace-period=0 cluster1-pxc-0 cluster1-pxc-1

8. Wait for the successful startup of the Pods which were deleted during the
   previous step, and finally remove the ``cluster1-pxc-2`` Pod:

   .. code-block:: bash

      $ kubectl delete pods --force --grace-period=0 cluster1-pxc-2

9. After the Pod startup, the cluster is fully recovered.
