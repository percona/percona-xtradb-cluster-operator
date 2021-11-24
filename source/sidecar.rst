.. _operator-sidecar:

================================================================================
Using sidecar containers
================================================================================

The Operator allows you to deploy additional (so-called *sidecar*) containers to
the Pod. You can use this feature to run debugging tools, some specific
monitoring solutions, etc.

.. note::  Custom sidecar containers `can easily access other components of your cluster <https://kubernetes.io/docs/concepts/workloads/pods/#resource-sharing-and-communication>`_. 
   Therefore they should be used carefully and by experienced users only.

Adding a sidecar container
==========================

You can add sidecar containers to Percona XtraDB Cluster, HAProxy, and ProxySQL
Pods. Just use ``sidecars`` subsection ing the ``pxc``, ``haproxy``, or
``proxysql`` section of the ``deploy/cr.yaml`` configuration file. In this
subsection, you should specify the name and image of your container and possibly
a command to run:

.. code:: yaml

   spec:
     pxc:
       ....
       sidecars:
       - image: busybox
         command: ["/bin/sh"]
         args: ["-c", "while true; do echo echo $(date -u) 'test' >> /dev/null; sleep 5; done"]
         name: my-sidecar-1
       ....

Apply your modifications as usual:

.. code:: bash

   $ kubectl apply -f deploy/cr.yaml

Running ``kubectl describe`` command for the appropriate Pod can bring you the
information about the newly created container:

.. code:: text

   $ kubectl describe pod cluster1-pxc-0
   ....
   Containers:
   ....
   my-sidecar-1:
     Container ID:  docker://f0c3437295d0ec819753c581aae174a0b8d062337f80897144eb8148249ba742
     Image:         busybox
     Image ID:      docker-pullable://busybox@sha256:139abcf41943b8bcd4bc5c42ee71ddc9402c7ad69ad9e177b0a9bc4541f14924
     Port:          <none>
     Host Port:     <none>
     Command:
       /bin/sh
     Args:
       -c
       while true; do echo echo $(date -u) 'test' >> /dev/null; sleep 5; done
     State:          Running
       Started:      Thu, 11 Nov 2021 10:38:15 +0300
     Ready:          True
     Restart Count:  0
     Environment:    <none>
     Mounts:
       /var/run/secrets/kubernetes.io/serviceaccount from kube-api-access-fbrbn (ro)
   ....

Getting shell access to a sidecar container
===========================================

You can login to your sidecar container as follows:

.. code:: bash

   $ kubectl exec -it cluster1-pxc-0 -c my-sidecar-1 -- sh
   / #

Mount volumes into sidecar containers
=====================================

It is possible to mount volumes into sidecar containers.

Following subsections describe different `volume types <https://kubernetes.io/docs/concepts/storage/volumes/#volume-types>`_,
which were tested with sidecar containers and are known to work.

Persistent Volume
-----------------

You can use `Persistent volumes <https://kubernetes.io/docs/concepts/storage/persistent-volumes/>`_ when you need dynamically provisioned storage which doesn't depend on the Pod lifecycle.
To use such volume, you should *claim* durable storage with `persistentVolumeClaim <https://kubernetes.io/docs/concepts/storage/volumes/#persistentvolumeclaim>`_ without specifying any non-important details.

The following example requests 1G storage with ``sidecar-volume-claim``
PersistentVolumeClaim, and mounts the correspondent Persistent Volume to the
``my-sidecar-1`` container's filesystem under the ``/volume1`` directory:

.. code:: yaml

   ...
   sidecars:
     - image: busybox
       command: ["/bin/sh"]
       args: ["-c", "while true; do echo echo $(date -u) 'test' >> /dev/null; sleep 5; done"]
       name: my-sidecar-1
       volumeMounts:
       - mountPath: /volume1
         name: sidecar-volume-claim
     sidecarPVCs:
     - apiVersion: v1
       kind: PersistentVolumeClaim
       metadata:
         name: sidecar-volume-claim
       spec:
         resources:
           requests:
             storage: 1Gi
         volumeMode: Filesystem
         accessModes:
           - ReadWriteOnce 

Secret
------

You can use a `secret volume <https://kubernetes.io/docs/concepts/storage/volumes/#secret>`_
to pass the information which needs additional protection (e.g. passwords), to
the container. Secrets are stored with the Kubernetes API and mounted to the
container as RAM-stored files.

You can mount a secret volume as follows:

.. code:: yaml

   ...
   sidecars:
     - image: busybox
       command: ["/bin/sh"]
       args: ["-c", "while true; do echo echo $(date -u) 'test' >> /dev/null; sleep 5; done"]
       name: my-sidecar-1
       volumeMounts:
       - mountPath: /secret
         name: sidecar-secret
     sidecarVolumes:
     - name: sidecar-secret
       secret:
         secretName: mysecret

The above example creates a ``sidecar-secret`` volume (based on already existing
``mysecret`` `Secret object <https://kubernetes.io/docs/concepts/configuration/secret/>`_)
and mounts it to the ``my-sidecar-1`` container's filesystem under the
``/secret`` directory.

.. note:: Don't forget you need to `create a Secret Object <https://kubernetes.io/docs/concepts/configuration/secret/>`_ before you can use it.

configMap
---------

You can use a `configMap volume <https://kubernetes.io/docs/concepts/storage/volumes/#configmap>`_ to pass some configuration data to the container.
Secrets are stored with the Kubernetes API and mounted to the container as RAM-stored files.

You can mount a configMap volume as follows:

.. code:: yaml

   ...
   sidecars:
     - image: busybox
       command: ["/bin/sh"]
       args: ["-c", "while true; do echo echo $(date -u) 'test' >> /dev/null; sleep 5; done"]
       name: my-sidecar-1
       volumeMounts:
       - mountPath: /config
         name: sidecar-config
     sidecarVolumes:
     - name: sidecar-config
       configMap:
         name: myconfigmap

The above example creates a ``sidecar-config`` volume (based on already existing
``myconfigmap`` `configMap object <https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/>`_)
and mounts it to the ``my-sidecar-1`` container's filesystem under the
``/config`` directory.

.. note:: Don't forget you need to `create a configMap Object <https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/#create-a-configmap>`_ before you can use it.
