.. _storage-local:

Local Storage support for the |operator|
=====================================================================

Among the wide rage of volume types, available in Kubernetes, there are
some which allow Pod containers to access part of the local filesystem on
the node. Two such options provided by Kubernetes itself are *emptyDir* and
*hostPath* volumes. More comprehensive setups require additional components,
such as `OpenEBS Container Attached Storage solution <https://openebs.io/>`_

.. _storage-emptydir:

emptyDir
--------

The name of this option is self-explanatory. When Pod having an
`emptyDir
volume <https://kubernetes.io/docs/concepts/storage/volumes/#emptydir>`__
is assigned to a Node, a directory with the specified name is created on
this node and exists until this Pod is removed from the node. When the
Pod have been deleted, the directory is deleted too with all its
content. All containers in the Pod which have mounted this volume will
gain read and write access to the correspondent directory.

The ``emptyDir`` options in the
`deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/main/deploy/cr.yaml>`__
file can be used to turn the emptyDir volume on by setting the directory
name.

.. _storage-hostpath:

hostPath
--------

A `hostPath
volume <https://kubernetes.io/docs/concepts/storage/volumes/#hostpath>`__
mounts some existing file or directory from the node’s filesystem into
the Pod.

The ``volumeSpec.hostPath`` subsection in the
`deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/main/deploy/cr.yaml>`__
file may include ``path`` and ``type`` keys to set the node’s filesystem
object path and to specify whether it is a file, a directory, or
something else (e.g. a socket):

::

    volumeSpec:
      hostPath:
        path: /data
        type: Directory

Please note, that hostPath directory is not created automatically! It
should be :ref:`created manually on the node's filesystem<faq-hostpath>`.
Also, it should have the attributives (access permissions, ownership, SELinux
security context) which would allow Pod to access the correspondent filesystem
objects according to :ref:`pxc.containerSecurityContext<pxc-containersecuritycontext>`
and :ref:`pxc.podSecurityContext<pxc-podsecuritycontext>`.

``hostPath`` is useful when you are able to perform manual actions
during the first run and have strong need in improved disk performance.
Also, please consider using tolerations to avoid cluster migration to
different hardware in case of a reboot or a hardware failure.

More details can be found in the `official hostPath Kubernetes
documentation <https://kubernetes.io/docs/concepts/storage/volumes/#hostpath>`__.

.. _storage-openebs:

OpenEBS Local Persistent Volume Hostpath
----------------------------------------

Both  *emptyDir* and *hostPath* volumes do not support `Dynamic Volume Provisioning <https://kubernetes.io/docs/concepts/storage/dynamic-provisioning/>`_.
Options that allow combining Dynamic Volume Provisioning with Local Persistent
Volumes are provided by `OpenEBS <https://openebs.io>`__. Particularly,
`OpenEBS Local PV Hostpath <https://openebs.io/docs/user-guides/localpv-hostpath>`_ allows creating Kubernetes Local Persistent Volumes
using a directory (Hostpath) on the node. Such volume can be further accessed by
applications via `Storage Class <https://kubernetes.io/docs/concepts/storage/storage-classes/>`_
and `PersistentVolumeClaim <https://kubernetes.io/docs/concepts/storage/persistent-volumes/>`_.

Using it involves the following steps.

#. Install OpenEBS on your system along with the official `installation guide <https://openebs.io/docs/user-guides/installation>`_.

#. Define a new `Kubernetes Storage Class <https://kubernetes.io/docs/concepts/storage/storage-classes/>`_
   with OpenEBS with the YAML file (e. g. ``local-hostpath.yaml``) as follows:
   
   .. code:: yaml
   
      apiVersion: storage.k8s.io/v1
      kind: StorageClass
      metadata:
        name: localpv
        annotations:
          openebs.io/cas-type: local
          cas.openebs.io/config: |
            - name: StorageType
              value: hostpath
            - name: BasePath
              value: /var/local-hostpath
      provisioner: openebs.io/local
      reclaimPolicy: Delete
      volumeBindingMode: WaitForFirstConsumer

   Two things to edit in this example are the ``metadata.name`` key (you will
   use it as a storage class name) and  the ``value`` option under the
   ``cas.openebs.io/config`` (it should point to an already existing directory
   on the local filesystem of your node).
   
   When ready, apply the file with the ``kubectl apply -f local-hostpath.yaml``
   command.
   
   #. Now you can deploy the Operator and Percona XtraDB Cluster using this
   StorageClass in ``deploy/cr.yaml``:
   
   .. code:: yaml
   
      ...
      volumeSpec:
         persistentVolumeClaim:
           storageClassName: localpv
           accessModes: [ "ReadWriteOnce" ]
           resources:
             requests:
               storage: 200Gi

.. note:: There are other storage options provided by the OpenEBS, which may
   be helpful within your cluster setup. Look at the `OpenEBS for the Management of Kubernetes Storage Volumes <https://www.percona.com/blog/2020/11/09/openebs-for-the-management-of-kubernetes-storage-volumes/>`_ blog post for more examples. Also, consider
   looking at the `Measuring OpenEBS Local Volume Performance Overhead in Kubernetes <https://www.percona.com/blog/2020/11/12/measuring-openebs-local-volume-performance-overhead-in-kubernetes/>`_ post.
