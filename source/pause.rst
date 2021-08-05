.. _operator-pause:

`Pause/resume Percona XtraDB Cluster <pause.html#pause>`_
===============================================================================

There may be external situations when it is needed to shutdown the Percona
XtraDB Cluster for a while and then start it back up (some works related to the
maintenance of the enterprise infrastructure, etc.).

The ``deploy/cr.yaml`` file contains a special ``spec.pause`` key for this.
Setting it to ``true`` gracefully stops the cluster:

.. code:: yaml

   spec:
     .......
     pause: true

Pausing the cluster may take some time, and when the process is over, you will
see only the Operator Pod running:

.. code:: bash

   $ kubectl get pods
   NAME                                               READY   STATUS    RESTARTS   AGE
   percona-xtradb-cluster-operator-79966668bd-rswbk   1/1     Running   0          12m

To start the cluster after it was shut down just revert the ``spec.pause`` key
to ``false``. 

Starting the cluster will take time. The process is over when all Pods have
reached their Running status:

.. include:: ./assets/code/kubectl-get-pods-response.txt
