Local Storage support for the Percona XtraDB Cluster Operator
===============================================================================

Among the wide rage of volume types, supported by Kubernetes, there are two which allow Pod containers to access part of the local filesystem on the node. Two such options are *emptyDir* and *hostPath* volumes.

emptyDir
-------------------------------------------------------------------------------

The name of this option is self-explanatory. When Pod having an [emptyDir volume](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir) is assigned to a Node, a directory with the specified name is created on this node and exists until this Pod is removed from the node. When the Pod have been deleted, the directory is deleted too with all its content. All containers in the Pod which have mounted this volume will gain read and write access to the correspondent directory.

The ``emptyDir`` options in the [deploy/cr.yaml](https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml) file can be used to turn the emptyDir volume on by setting the directory name.

hostPath
-------------------------------------------------------------------------------

A [hostPath volume](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath) mounts some existing file or directory from the node’s filesystem into the Pod.

The ``volumeSpec.hostPath`` subsection in the [deploy/cr.yaml](https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml) file may include ``path`` and ``type`` keys to set the node's filesystem object path and to specify whether it is a file, a directory, or something else (e.g. a socket):

   ```
    volumeSpec:
      hostPath:
        path: /data
        type: Directory
   ```

Please note, that hostPath directory is not created automatically! Is should be created manually and should have following correct attributives: 
1. access permissions 
2. ownership
3. SELinux security context

Skipping any of these three can lead to a malfunction that will be difficult to debug.

More details can be found in the [official hostPath Kubernetes documentation](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath).
