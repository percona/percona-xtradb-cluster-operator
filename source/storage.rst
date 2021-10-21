.. _storage-local:

Local Storage support for the Percona Distribution for MySQL Operator
=====================================================================

Among the wide rage of volume types, supported by Kubernetes, there are
two which allow Pod containers to access part of the local filesystem on
the node. Two such options are *emptyDir* and *hostPath* volumes.

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
`deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml>`__
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
`deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml>`__
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
