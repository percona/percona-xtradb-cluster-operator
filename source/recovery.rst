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

Obviously, these continuous restarts prevent to get ssh access to the container,
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
