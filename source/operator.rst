Custom Resource options
=======================

The operator is configured via the spec section of the
`deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml>`__
file. This file contains the following spec sections to configure three
main subsystems of the cluster:

======== ========== =========================================
Key      Value Type Description
======== ========== =========================================
pxc      subdoc     Percona XtraDB Cluster general section
proxysql subdoc     ProxySQL section
pmm      subdoc     Percona Monitoring and Management section
backup   subdoc     Percona XtraDB Cluster backups section
======== ========== =========================================

PXC Section
-----------

The ``pxc`` section in the deploy/cr.yaml file contains general
configuration options for the Percona XtraDB Cluster.

+--------------------------------+-----------+----------+------------+
| Key                            | Value     | Example  | Descriptio |
|                                | Type      |          | n          |
+================================+===========+==========+============+
| size                           | int       | ``3``    | The size   |
|                                |           |          | of the     |
|                                |           |          | Percona    |
|                                |           |          | XtraDB     |
|                                |           |          | Cluster,   |
|                                |           |          | must be >= |
|                                |           |          | 3 for      |
|                                |           |          | `High-Avai |
|                                |           |          | lability < |
|                                |           |          | hhttps://w |
|                                |           |          | ww.percona |
|                                |           |          | .com/doc/p |
|                                |           |          | ercona-xtr |
|                                |           |          | adb-cluste |
|                                |           |          | r/5.7/intr |
|                                |           |          | o.html>`__ |
+--------------------------------+-----------+----------+------------+
| image                          | string    | ``percon | Percona    |
|                                |           | alab/pxc | XtraDB     |
|                                |           | -openshi | Cluster    |
|                                |           | ft:0.1.0 | docker     |
|                                |           | ``       | image to   |
|                                |           |          | use        |
+--------------------------------+-----------+----------+------------+
| configuration                  | string    | \|\ ``[m | The        |
|                                |           | ysqld]`` | ``my.cnf`` |
|                                |           | \ \ ``ws | file       |
|                                |           | rep_debu | options    |
|                                |           | g=ON``\  | which are  |
|                                |           | \ ``[sst | to be      |
|                                |           | ]``\ \ ` | passed to  |
|                                |           | `wsrep_d | Percona    |
|                                |           | ebug=ON` | XtraDB     |
|                                |           | `        | Cluster    |
|                                |           |          | nodes      |
+--------------------------------+-----------+----------+------------+
| resources.requests.memory      | string    | ``1G``   | `Kubernete |
|                                |           |          | s          |
|                                |           |          | Memory     |
|                                |           |          | requests < |
|                                |           |          | https://ku |
|                                |           |          | bernetes.i |
|                                |           |          | o/docs/con |
|                                |           |          | cepts/conf |
|                                |           |          | iguration/ |
|                                |           |          | manage-com |
|                                |           |          | pute-resou |
|                                |           |          | rces-conta |
|                                |           |          | iner/#reso |
|                                |           |          | urce-reque |
|                                |           |          | sts-and-li |
|                                |           |          | mits-of-po |
|                                |           |          | d-and-cont |
|                                |           |          | ainer>`__  |
|                                |           |          | for a PXC  |
|                                |           |          | container  |
+--------------------------------+-----------+----------+------------+
| resources.requests.cpu         | string    | ``600m`` | `Kubernete |
|                                |           |          | s          |
|                                |           |          | CPU        |
|                                |           |          | requests < |
|                                |           |          | https://ku |
|                                |           |          | bernetes.i |
|                                |           |          | o/docs/con |
|                                |           |          | cepts/conf |
|                                |           |          | iguration/ |
|                                |           |          | manage-com |
|                                |           |          | pute-resou |
|                                |           |          | rces-conta |
|                                |           |          | iner/#reso |
|                                |           |          | urce-reque |
|                                |           |          | sts-and-li |
|                                |           |          | mits-of-po |
|                                |           |          | d-and-cont |
|                                |           |          | ainer>`__  |
|                                |           |          | for a PXC  |
|                                |           |          | container  |
+--------------------------------+-----------+----------+------------+
| resources.limits.memory        | string    | ``1G``   | `Kubernete |
|                                |           |          | s          |
|                                |           |          | Memory     |
|                                |           |          | limit <htt |
|                                |           |          | ps://kuber |
|                                |           |          | netes.io/d |
|                                |           |          | ocs/concep |
|                                |           |          | ts/configu |
|                                |           |          | ration/man |
|                                |           |          | age-comput |
|                                |           |          | e-resource |
|                                |           |          | s-containe |
|                                |           |          | r/#resourc |
|                                |           |          | e-requests |
|                                |           |          | -and-limit |
|                                |           |          | s-of-pod-a |
|                                |           |          | nd-contain |
|                                |           |          | er>`__     |
|                                |           |          | for a PXC  |
|                                |           |          | container  |
+--------------------------------+-----------+----------+------------+
| resources.limits.cpu           | string    | ``1``    | `Kubernete |
|                                |           |          | s          |
|                                |           |          | CPU        |
|                                |           |          | limit <htt |
|                                |           |          | ps://kuber |
|                                |           |          | netes.io/d |
|                                |           |          | ocs/concep |
|                                |           |          | ts/configu |
|                                |           |          | ration/man |
|                                |           |          | age-comput |
|                                |           |          | e-resource |
|                                |           |          | s-containe |
|                                |           |          | r/#resourc |
|                                |           |          | e-requests |
|                                |           |          | -and-limit |
|                                |           |          | s-of-pod-a |
|                                |           |          | nd-contain |
|                                |           |          | er>`__     |
|                                |           |          | for a PXC  |
|                                |           |          | container  |
+--------------------------------+-----------+----------+------------+
| volumeSpec.emptyDir            | string    | ``{}``   | `Kubernete |
|                                |           |          | s          |
|                                |           |          | emptyDir   |
|                                |           |          | volume <ht |
|                                |           |          | tps://kube |
|                                |           |          | rnetes.io/ |
|                                |           |          | docs/conce |
|                                |           |          | pts/storag |
|                                |           |          | e/volumes/ |
|                                |           |          | #emptydir> |
|                                |           |          | `__,       |
|                                |           |          | i.e. the   |
|                                |           |          | directory  |
|                                |           |          | which will |
|                                |           |          | be created |
|                                |           |          | on a node, |
|                                |           |          | and will   |
|                                |           |          | be         |
|                                |           |          | accessible |
|                                |           |          | to the PXC |
|                                |           |          | Pod        |
|                                |           |          | containers |
+--------------------------------+-----------+----------+------------+
| volumeSpec.hostPath.path       | string    | ``/data` | `Kubernete |
|                                |           | `        | s          |
|                                |           |          | hostPath   |
|                                |           |          | volume <ht |
|                                |           |          | tps://kube |
|                                |           |          | rnetes.io/ |
|                                |           |          | docs/conce |
|                                |           |          | pts/storag |
|                                |           |          | e/volumes/ |
|                                |           |          | #hostpath> |
|                                |           |          | `__,       |
|                                |           |          | i.e. the   |
|                                |           |          | file or    |
|                                |           |          | directory  |
|                                |           |          | of a node  |
|                                |           |          | that will  |
|                                |           |          | be         |
|                                |           |          | accessible |
|                                |           |          | to the PXC |
|                                |           |          | Pod        |
|                                |           |          | containers |
+--------------------------------+-----------+----------+------------+
| volumeSpec.hostPath.type       | string    | ``Direct | The        |
|                                |           | ory``    | `Kubernete |
|                                |           |          | s          |
|                                |           |          | hostPath   |
|                                |           |          | volume     |
|                                |           |          | type <http |
|                                |           |          | s://kubern |
|                                |           |          | etes.io/do |
|                                |           |          | cs/concept |
|                                |           |          | s/storage/ |
|                                |           |          | volumes/#h |
|                                |           |          | ostpath>`_ |
|                                |           |          | _          |
+--------------------------------+-----------+----------+------------+
| volumeSpec.persistentVolumeCla | string    | ``standa | Set the    |
| im.storageClassName            |           | rd``     | `Kubernete |
|                                |           |          | s          |
|                                |           |          | Storage    |
|                                |           |          | Class <htt |
|                                |           |          | ps://kuber |
|                                |           |          | netes.io/d |
|                                |           |          | ocs/concep |
|                                |           |          | ts/storage |
|                                |           |          | /storage-c |
|                                |           |          | lasses/>`_ |
|                                |           |          | _          |
|                                |           |          | to use     |
|                                |           |          | with the   |
|                                |           |          | PXC        |
|                                |           |          | `Persisten |
|                                |           |          | t          |
|                                |           |          | Volume     |
|                                |           |          | Claim <htt |
|                                |           |          | ps://kuber |
|                                |           |          | netes.io/d |
|                                |           |          | ocs/concep |
|                                |           |          | ts/storage |
|                                |           |          | /persisten |
|                                |           |          | t-volumes/ |
|                                |           |          | #persisten |
|                                |           |          | tvolumecla |
|                                |           |          | ims>`__    |
+--------------------------------+-----------+----------+------------+
| volumeSpec.persistentVolumeCla | array     | ``[ "Rea | `Kubernete |
| im.accessModes                 |           | dWriteOn | s          |
|                                |           | ce" ]``  | Persistent |
|                                |           |          | Volume <ht |
|                                |           |          | tps://kube |
|                                |           |          | rnetes.io/ |
|                                |           |          | docs/conce |
|                                |           |          | pts/storag |
|                                |           |          | e/persiste |
|                                |           |          | nt-volumes |
|                                |           |          | />`__      |
|                                |           |          | access     |
|                                |           |          | modes for  |
|                                |           |          | the        |
|                                |           |          | PerconaXtr |
|                                |           |          | aDB        |
|                                |           |          | Cluster    |
+--------------------------------+-----------+----------+------------+
| volumeSpec.resources.requests. | string    | ``6Gi``  | The        |
| storage                        |           |          | `Kubernete |
|                                |           |          | s          |
|                                |           |          | Persistent |
|                                |           |          | Volume <ht |
|                                |           |          | tps://kube |
|                                |           |          | rnetes.io/ |
|                                |           |          | docs/conce |
|                                |           |          | pts/storag |
|                                |           |          | e/persiste |
|                                |           |          | nt-volumes |
|                                |           |          | />`__      |
|                                |           |          | size for   |
|                                |           |          | the        |
|                                |           |          | Percona    |
|                                |           |          | XtraDB     |
|                                |           |          | Cluster    |
+--------------------------------+-----------+----------+------------+
| affinity.topologyKey           | string    | ``kubern | The        |
|                                |           | etes.io/ | `Operator  |
|                                |           | hostname | topologyKe |
|                                |           | ``       | y <./const |
|                                |           |          | raints>`__ |
|                                |           |          | node       |
|                                |           |          | anti-affin |
|                                |           |          | ity        |
|                                |           |          | constraint |
+--------------------------------+-----------+----------+------------+
| affinity.advanced              | subdoc    |          | If         |
|                                |           |          | available, |
|                                |           |          | it makes   |
|                                |           |          | `topologyK |
|                                |           |          | ey <https: |
|                                |           |          | //kubernet |
|                                |           |          | es.io/docs |
|                                |           |          | /concepts/ |
|                                |           |          | configurat |
|                                |           |          | ion/assign |
|                                |           |          | -pod-node/ |
|                                |           |          | #inter-pod |
|                                |           |          | -affinity- |
|                                |           |          | and-anti-a |
|                                |           |          | ffinity-be |
|                                |           |          | ta-feature |
|                                |           |          | >`__       |
|                                |           |          | node       |
|                                |           |          | affinity   |
|                                |           |          | constraint |
|                                |           |          | to be      |
|                                |           |          | ignored    |
+--------------------------------+-----------+----------+------------+
| nodeSelector                   | label     | ``diskty | The        |
|                                |           | pe: ssd` | `Kubernete |
|                                |           | `        | s          |
|                                |           |          | nodeSelect |
|                                |           |          | or <https: |
|                                |           |          | //kubernet |
|                                |           |          | es.io/docs |
|                                |           |          | /concepts/ |
|                                |           |          | configurat |
|                                |           |          | ion/assign |
|                                |           |          | -pod-node/ |
|                                |           |          | #nodeselec |
|                                |           |          | tor>`__    |
|                                |           |          | constraint |
+--------------------------------+-----------+----------+------------+
| tolerations                    | subdoc    | ``node.a | The        |
|                                |           | lpha.kub | [Kubernete |
|                                |           | ernetes. | s          |
|                                |           | io/unrea | Pod        |
|                                |           | chable`` | toleration |
|                                |           |          | s]         |
|                                |           |          | (https://k |
|                                |           |          | ubernetes. |
|                                |           |          | io/docs/co |
|                                |           |          | ncepts/con |
|                                |           |          | figuration |
|                                |           |          | /taint-and |
|                                |           |          | -toleratio |
|                                |           |          | n/#concept |
|                                |           |          | s)         |
+--------------------------------+-----------+----------+------------+
| priorityClassName              | string    | ``high-p | The        |
|                                |           | riority` | `Kuberente |
|                                |           | `        | s          |
|                                |           |          | Pod        |
|                                |           |          | priority   |
|                                |           |          | class <htt |
|                                |           |          | ps://kuber |
|                                |           |          | netes.io/d |
|                                |           |          | ocs/concep |
|                                |           |          | ts/configu |
|                                |           |          | ration/pod |
|                                |           |          | -priority- |
|                                |           |          | preemption |
|                                |           |          | /#priority |
|                                |           |          | class>`__  |
+--------------------------------+-----------+----------+------------+
| annotations                    | label     | ``iam.am | The        |
|                                |           | azonaws. | `Kubernete |
|                                |           | com/role | s          |
|                                |           | : role-a | annotation |
|                                |           | rn``     | s <https:/ |
|                                |           |          | /kubernete |
|                                |           |          | s.io/docs/ |
|                                |           |          | concepts/o |
|                                |           |          | verview/wo |
|                                |           |          | rking-with |
|                                |           |          | -objects/a |
|                                |           |          | nnotations |
|                                |           |          | />`__      |
|                                |           |          | metadata   |
+--------------------------------+-----------+----------+------------+
| imagePullSecrets.name          | string    | ``privat | `Kubernete |
|                                |           | e-regist | s          |
|                                |           | ry-crede | imagePullS |
|                                |           | ntials`` | ecret <htt |
|                                |           |          | ps://kuber |
|                                |           |          | netes.io/d |
|                                |           |          | ocs/concep |
|                                |           |          | ts/configu |
|                                |           |          | ration/sec |
|                                |           |          | ret/#using |
|                                |           |          | -imagepull |
|                                |           |          | secrets>`_ |
|                                |           |          | _          |
|                                |           |          | for the    |
|                                |           |          | Percona    |
|                                |           |          | XtraDB     |
|                                |           |          | Cluster    |
|                                |           |          | docker     |
|                                |           |          | image      |
+--------------------------------+-----------+----------+------------+
| labels                         | label     | ``rack:  | The        |
|                                |           | rack-22` | `Kubernete |
|                                |           | `        | s          |
|                                |           |          | affinity   |
|                                |           |          | labels <ht |
|                                |           |          | tps://kube |
|                                |           |          | rnetes.io/ |
|                                |           |          | docs/conce |
|                                |           |          | pts/config |
|                                |           |          | uration/as |
|                                |           |          | sign-pod-n |
|                                |           |          | ode/>`__   |
+--------------------------------+-----------+----------+------------+

ProxySQL Section
----------------

The ``proxysql`` section in the deploy/cr.yaml file contains
configuration options for the ProxySQL daemon.

+--------------------------------+-----------+----------+------------+
| Key                            | Value     | Example  | Descriptio |
|                                | Type      |          | n          |
+================================+===========+==========+============+
| enabled                        | boolean   | ``true`` | Enables or |
|                                |           |          | disables   |
|                                |           |          | `load      |
|                                |           |          | balancing  |
|                                |           |          | with       |
|                                |           |          | ProxySQL < |
|                                |           |          | https://ww |
|                                |           |          | w.percona. |
|                                |           |          | com/doc/pe |
|                                |           |          | rcona-xtra |
|                                |           |          | db-cluster |
|                                |           |          | /5.7/howto |
|                                |           |          | s/proxysql |
|                                |           |          | .html>`__  |
|                                |           |          | `Service < |
|                                |           |          | https://ku |
|                                |           |          | bernetes.i |
|                                |           |          | o/docs/con |
|                                |           |          | cepts/serv |
|                                |           |          | ices-netwo |
|                                |           |          | rking/serv |
|                                |           |          | ice/>`__   |
+--------------------------------+-----------+----------+------------+
| size                           | int       | ``1``    | The number |
|                                |           |          | of the     |
|                                |           |          | ProxySQL   |
|                                |           |          | daemons    |
|                                |           |          | `to        |
|                                |           |          | provide    |
|                                |           |          | load       |
|                                |           |          | balancing  |
|                                |           |          | <https://w |
|                                |           |          | ww.percona |
|                                |           |          | .com/doc/p |
|                                |           |          | ercona-xtr |
|                                |           |          | adb-cluste |
|                                |           |          | r/5.7/howt |
|                                |           |          | os/proxysq |
|                                |           |          | l.html>`__ |
|                                |           |          | ,          |
|                                |           |          | must be =  |
|                                |           |          | 1 in       |
|                                |           |          | current    |
|                                |           |          | release    |
+--------------------------------+-----------+----------+------------+
| image                          | string    | ``percon | ProxySQL   |
|                                |           | alab/pro | docker     |
|                                |           | xysql-op | image to   |
|                                |           | enshift: | use        |
|                                |           | 0.1.0``  |            |
+--------------------------------+-----------+----------+------------+
| resources.requests.memory      | string    | ``1G``   | `Kubernete |
|                                |           |          | s          |
|                                |           |          | Memory     |
|                                |           |          | requests < |
|                                |           |          | https://ku |
|                                |           |          | bernetes.i |
|                                |           |          | o/docs/con |
|                                |           |          | cepts/conf |
|                                |           |          | iguration/ |
|                                |           |          | manage-com |
|                                |           |          | pute-resou |
|                                |           |          | rces-conta |
|                                |           |          | iner/#reso |
|                                |           |          | urce-reque |
|                                |           |          | sts-and-li |
|                                |           |          | mits-of-po |
|                                |           |          | d-and-cont |
|                                |           |          | ainer>`__  |
|                                |           |          | for a      |
|                                |           |          | ProxySQL   |
|                                |           |          | container  |
+--------------------------------+-----------+----------+------------+
| resources.requests.cpu         | string    | ``600m`` | `Kubernete |
|                                |           |          | s          |
|                                |           |          | CPU        |
|                                |           |          | requests < |
|                                |           |          | https://ku |
|                                |           |          | bernetes.i |
|                                |           |          | o/docs/con |
|                                |           |          | cepts/conf |
|                                |           |          | iguration/ |
|                                |           |          | manage-com |
|                                |           |          | pute-resou |
|                                |           |          | rces-conta |
|                                |           |          | iner/#reso |
|                                |           |          | urce-reque |
|                                |           |          | sts-and-li |
|                                |           |          | mits-of-po |
|                                |           |          | d-and-cont |
|                                |           |          | ainer>`__  |
|                                |           |          | for a      |
|                                |           |          | ProxySQL   |
|                                |           |          | container  |
+--------------------------------+-----------+----------+------------+
| resources.limits.memory        | string    | ``1G``   | `Kubernete |
|                                |           |          | s          |
|                                |           |          | Memory     |
|                                |           |          | limit <htt |
|                                |           |          | ps://kuber |
|                                |           |          | netes.io/d |
|                                |           |          | ocs/concep |
|                                |           |          | ts/configu |
|                                |           |          | ration/man |
|                                |           |          | age-comput |
|                                |           |          | e-resource |
|                                |           |          | s-containe |
|                                |           |          | r/#resourc |
|                                |           |          | e-requests |
|                                |           |          | -and-limit |
|                                |           |          | s-of-pod-a |
|                                |           |          | nd-contain |
|                                |           |          | er>`__     |
|                                |           |          | for a      |
|                                |           |          | ProxySQL   |
|                                |           |          | container  |
+--------------------------------+-----------+----------+------------+
| resources.limits.cpu           | string    | ``700m`` | `Kubernete |
|                                |           |          | s          |
|                                |           |          | CPU        |
|                                |           |          | limit <htt |
|                                |           |          | ps://kuber |
|                                |           |          | netes.io/d |
|                                |           |          | ocs/concep |
|                                |           |          | ts/configu |
|                                |           |          | ration/man |
|                                |           |          | age-comput |
|                                |           |          | e-resource |
|                                |           |          | s-containe |
|                                |           |          | r/#resourc |
|                                |           |          | e-requests |
|                                |           |          | -and-limit |
|                                |           |          | s-of-pod-a |
|                                |           |          | nd-contain |
|                                |           |          | er>`__     |
|                                |           |          | for a      |
|                                |           |          | ProxySQL   |
|                                |           |          | container  |
+--------------------------------+-----------+----------+------------+
| volumeSpec.emptyDir            | string    | ``{}``   | `Kubernete |
|                                |           |          | s          |
|                                |           |          | emptyDir   |
|                                |           |          | volume <ht |
|                                |           |          | tps://kube |
|                                |           |          | rnetes.io/ |
|                                |           |          | docs/conce |
|                                |           |          | pts/storag |
|                                |           |          | e/volumes/ |
|                                |           |          | #emptydir> |
|                                |           |          | `__,       |
|                                |           |          | i.e. the   |
|                                |           |          | directory  |
|                                |           |          | which will |
|                                |           |          | be created |
|                                |           |          | on a node, |
|                                |           |          | and will   |
|                                |           |          | be         |
|                                |           |          | accessible |
|                                |           |          | to the     |
|                                |           |          | ProxySQL   |
|                                |           |          | Pod        |
|                                |           |          | containers |
+--------------------------------+-----------+----------+------------+
| volumeSpec.hostPath.path       | string    | ``/data` | `Kubernete |
|                                |           | `        | s          |
|                                |           |          | hostPath   |
|                                |           |          | volume <ht |
|                                |           |          | tps://kube |
|                                |           |          | rnetes.io/ |
|                                |           |          | docs/conce |
|                                |           |          | pts/storag |
|                                |           |          | e/volumes/ |
|                                |           |          | #hostpath> |
|                                |           |          | `__,       |
|                                |           |          | i.e. the   |
|                                |           |          | file or    |
|                                |           |          | directory  |
|                                |           |          | of a node  |
|                                |           |          | that will  |
|                                |           |          | be         |
|                                |           |          | accessible |
|                                |           |          | to the     |
|                                |           |          | ProxySQL   |
|                                |           |          | Pod        |
|                                |           |          | containers |
+--------------------------------+-----------+----------+------------+
| volumeSpec.hostPath.type       | string    | ``Direct | The        |
|                                |           | ory``    | `Kubernete |
|                                |           |          | s          |
|                                |           |          | hostPath   |
|                                |           |          | volume     |
|                                |           |          | type <http |
|                                |           |          | s://kubern |
|                                |           |          | etes.io/do |
|                                |           |          | cs/concept |
|                                |           |          | s/storage/ |
|                                |           |          | volumes/#h |
|                                |           |          | ostpath>`_ |
|                                |           |          | _          |
+--------------------------------+-----------+----------+------------+
| volumeSpec.persistentVolumeCla | string    | ``standa | The        |
| im.storageClassName            |           | rd``     | `Kubernete |
|                                |           |          | s          |
|                                |           |          | Storage    |
|                                |           |          | Class <htt |
|                                |           |          | ps://kuber |
|                                |           |          | netes.io/d |
|                                |           |          | ocs/concep |
|                                |           |          | ts/storage |
|                                |           |          | /storage-c |
|                                |           |          | lasses/>`_ |
|                                |           |          | _          |
|                                |           |          | to use     |
|                                |           |          | with the   |
|                                |           |          | ProxySQL   |
|                                |           |          | `Persisten |
|                                |           |          | t          |
|                                |           |          | Volume     |
|                                |           |          | Claim <htt |
|                                |           |          | ps://kuber |
|                                |           |          | netes.io/d |
|                                |           |          | ocs/concep |
|                                |           |          | ts/storage |
|                                |           |          | /persisten |
|                                |           |          | t-volumes/ |
|                                |           |          | #persisten |
|                                |           |          | tvolumecla |
|                                |           |          | ims>`__    |
+--------------------------------+-----------+----------+------------+
| volumeSpec.persistentVolumeCla | array     | ``[ "Rea | `Kubernete |
| im.accessModes                 |           | dWriteOn | s          |
|                                |           | ce" ]``  | Persistent |
|                                |           |          | Volume <ht |
|                                |           |          | tps://kube |
|                                |           |          | rnetes.io/ |
|                                |           |          | docs/conce |
|                                |           |          | pts/storag |
|                                |           |          | e/persiste |
|                                |           |          | nt-volumes |
|                                |           |          | />`__      |
|                                |           |          | access     |
|                                |           |          | modes for  |
|                                |           |          | ProxySQL   |
+--------------------------------+-----------+----------+------------+
| volumeSpec.resources.requests. | string    | ``2Gi``  | The        |
| storage                        |           |          | `Kubernete |
|                                |           |          | s          |
|                                |           |          | Persistent |
|                                |           |          | Volume <ht |
|                                |           |          | tps://kube |
|                                |           |          | rnetes.io/ |
|                                |           |          | docs/conce |
|                                |           |          | pts/storag |
|                                |           |          | e/persiste |
|                                |           |          | nt-volumes |
|                                |           |          | />`__      |
|                                |           |          | size for   |
|                                |           |          | ProxySQL   |
+--------------------------------+-----------+----------+------------+
| affinity.topologyKey           | string    | ``failur | The        |
|                                |           | e-domain | `Operator  |
|                                |           | .beta.ku | topologyKe |
|                                |           | bernetes | y <./const |
|                                |           | .io/zone | raints>`__ |
|                                |           | ``       | node       |
|                                |           |          | anti-affin |
|                                |           |          | ity        |
|                                |           |          | constraint |
+--------------------------------+-----------+----------+------------+
| affinity.advanced              | subdoc    |          | If         |
|                                |           |          | available, |
|                                |           |          | it makes   |
|                                |           |          | `topologyK |
|                                |           |          | ey <https: |
|                                |           |          | //kubernet |
|                                |           |          | es.io/docs |
|                                |           |          | /concepts/ |
|                                |           |          | configurat |
|                                |           |          | ion/assign |
|                                |           |          | -pod-node/ |
|                                |           |          | #inter-pod |
|                                |           |          | -affinity- |
|                                |           |          | and-anti-a |
|                                |           |          | ffinity-be |
|                                |           |          | ta-feature |
|                                |           |          | >`__       |
|                                |           |          | node       |
|                                |           |          | affinity   |
|                                |           |          | constraint |
|                                |           |          | to be      |
|                                |           |          | ignored    |
+--------------------------------+-----------+----------+------------+
| nodeSelector                   | label     | ``diskty | The        |
|                                |           | pe: ssd` | `Kubernete |
|                                |           | `        | s          |
|                                |           |          | nodeSelect |
|                                |           |          | or <https: |
|                                |           |          | //kubernet |
|                                |           |          | es.io/docs |
|                                |           |          | /concepts/ |
|                                |           |          | configurat |
|                                |           |          | ion/assign |
|                                |           |          | -pod-node/ |
|                                |           |          | #nodeselec |
|                                |           |          | tor>`__    |
|                                |           |          | affinity   |
|                                |           |          | constraint |
+--------------------------------+-----------+----------+------------+
| tolerations                    | subdoc    | ``node.a | The        |
|                                |           | lpha.kub | [Kubernete |
|                                |           | ernetes. | s          |
|                                |           | io/unrea | Pod        |
|                                |           | chable`` | toleration |
|                                |           |          | s]         |
|                                |           |          | (https://k |
|                                |           |          | ubernetes. |
|                                |           |          | io/docs/co |
|                                |           |          | ncepts/con |
|                                |           |          | figuration |
|                                |           |          | /taint-and |
|                                |           |          | -toleratio |
|                                |           |          | n/#concept |
|                                |           |          | s)         |
+--------------------------------+-----------+----------+------------+
| priorityClassName              | string    | ``high-p | The        |
|                                |           | riority` | `Kuberente |
|                                |           | `        | s          |
|                                |           |          | Pod        |
|                                |           |          | priority   |
|                                |           |          | class <htt |
|                                |           |          | ps://kuber |
|                                |           |          | netes.io/d |
|                                |           |          | ocs/concep |
|                                |           |          | ts/configu |
|                                |           |          | ration/pod |
|                                |           |          | -priority- |
|                                |           |          | preemption |
|                                |           |          | /#priority |
|                                |           |          | class>`__  |
|                                |           |          | for        |
|                                |           |          | ProxySQL   |
+--------------------------------+-----------+----------+------------+
| annotations                    | label     | ``iam.am | The        |
|                                |           | azonaws. | `Kubernete |
|                                |           | com/role | s          |
|                                |           | : role-a | annotation |
|                                |           | rn``     | s <https:/ |
|                                |           |          | /kubernete |
|                                |           |          | s.io/docs/ |
|                                |           |          | concepts/o |
|                                |           |          | verview/wo |
|                                |           |          | rking-with |
|                                |           |          | -objects/a |
|                                |           |          | nnotations |
|                                |           |          | />`__      |
|                                |           |          | metadata   |
+--------------------------------+-----------+----------+------------+
| imagePullSecrets.name          | string    | ``privat | `Kubernete |
|                                |           | e-regist | s          |
|                                |           | ry-crede | imagePullS |
|                                |           | ntials`` | ecret <htt |
|                                |           |          | ps://kuber |
|                                |           |          | netes.io/d |
|                                |           |          | ocs/concep |
|                                |           |          | ts/configu |
|                                |           |          | ration/sec |
|                                |           |          | ret/#using |
|                                |           |          | -imagepull |
|                                |           |          | secrets>`_ |
|                                |           |          | _          |
|                                |           |          | for the    |
|                                |           |          | ProxySQL   |
|                                |           |          | docker     |
|                                |           |          | image      |
+--------------------------------+-----------+----------+------------+
| labels                         | label     | ``rack:  | The        |
|                                |           | rack-22` | `Kubernete |
|                                |           | `        | s          |
|                                |           |          | affinity   |
|                                |           |          | labels <ht |
|                                |           |          | tps://kube |
|                                |           |          | rnetes.io/ |
|                                |           |          | docs/conce |
|                                |           |          | pts/config |
|                                |           |          | uration/as |
|                                |           |          | sign-pod-n |
|                                |           |          | ode/>`__   |
+--------------------------------+-----------+----------+------------+

PMM Section
-----------

The ``pmm`` section in the deploy/cr.yaml file contains configuration
options for Percona Monitoring and Management.

+---------+----------+--------------------+----------------------------+
| Key     | Value    | Example            | Description                |
|         | Type     |                    |                            |
+=========+==========+====================+============================+
| enabled | boolean  | ``false``          | Enables or disables        |
|         |          |                    | `monitoring Percona XtraDB |
|         |          |                    | Cluster with               |
|         |          |                    | PMM <https://www.percona.c |
|         |          |                    | om/doc/percona-xtradb-clus |
|         |          |                    | ter/LATEST/manual/monitori |
|         |          |                    | ng.html#using-pmm>`__      |
+---------+----------+--------------------+----------------------------+
| image   | string   | ``perconalab/pmm-c | PMM Client docker image to |
|         |          | lient``            | use                        |
+---------+----------+--------------------+----------------------------+
| serverH | string   | ``monitoring-servi | Address of the PMM Server  |
| ost     |          | ce``               | to collect data from the   |
|         |          |                    | Cluster                    |
+---------+----------+--------------------+----------------------------+
| serverU | string   | ``pmm``            | The `PMM Server            |
| ser     |          |                    | user <https://www.percona. |
|         |          |                    | com/doc/percona-monitoring |
|         |          |                    | -and-management/glossary.o |
|         |          |                    | ption.html#term-server-use |
|         |          |                    | r>`__.                     |
|         |          |                    | The PMM Server Password    |
|         |          |                    | should be configured via   |
|         |          |                    | secrets.                   |
+---------+----------+--------------------+----------------------------+

backup section
--------------

The ``backup`` section in the
`deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml>`__
file contains the following configuration options for the regular
Percona XtraDB Cluster backups.

+--------------------------------+-----------+----------+------------+
| Key                            | Value     | Example  | Descriptio |
|                                | Type      |          | n          |
+================================+===========+==========+============+
| image                          | string    | ``percon | Percona    |
|                                |           | alab/bac | XtraDB     |
|                                |           | kupjob-o | Cluster    |
|                                |           | penshift | docker     |
|                                |           | :0.2.0`` | image to   |
|                                |           |          | use for    |
|                                |           |          | the backup |
|                                |           |          | functional |
|                                |           |          | ity        |
+--------------------------------+-----------+----------+------------+
| imagePullSecrets.name          | string    | ``privat | `Kubernete |
|                                |           | e-regist | s          |
|                                |           | ry-crede | imagePullS |
|                                |           | ntials`` | ecret <htt |
|                                |           |          | ps://kuber |
|                                |           |          | netes.io/d |
|                                |           |          | ocs/concep |
|                                |           |          | ts/configu |
|                                |           |          | ration/sec |
|                                |           |          | ret/#using |
|                                |           |          | -imagepull |
|                                |           |          | secrets>`_ |
|                                |           |          | _          |
|                                |           |          | for the    |
|                                |           |          | specified  |
|                                |           |          | docker     |
|                                |           |          | image      |
+--------------------------------+-----------+----------+------------+
| storages.type                  | string    | ``s3``   | Type of    |
|                                |           |          | the cloud  |
|                                |           |          | storage to |
|                                |           |          | be used    |
|                                |           |          | for        |
|                                |           |          | backups.   |
|                                |           |          | Currently  |
|                                |           |          | only       |
|                                |           |          | ``s3`` and |
|                                |           |          | ``filesyst |
|                                |           |          | em``       |
|                                |           |          | types are  |
|                                |           |          | supported  |
+--------------------------------+-----------+----------+------------+
| storages.s3.credentialsSecret  | string    | ``my-clu | `Kubernete |
|                                |           | ster-nam | s          |
|                                |           | e-backup | secret <ht |
|                                |           | -s3``    | tps://kube |
|                                |           |          | rnetes.io/ |
|                                |           |          | docs/conce |
|                                |           |          | pts/config |
|                                |           |          | uration/se |
|                                |           |          | cret/>`__  |
|                                |           |          | for        |
|                                |           |          | backups.   |
|                                |           |          | It should  |
|                                |           |          | contain    |
|                                |           |          | ``AWS_ACCE |
|                                |           |          | SS_KEY_ID` |
|                                |           |          | `          |
|                                |           |          | and        |
|                                |           |          | ``AWS_SECR |
|                                |           |          | ET_ACCESS_ |
|                                |           |          | KEY``      |
|                                |           |          | keys.      |
+--------------------------------+-----------+----------+------------+
| storages.s3.bucket             | string    |          | The        |
|                                |           |          | `Amazon S3 |
|                                |           |          | bucket <ht |
|                                |           |          | tps://docs |
|                                |           |          | .aws.amazo |
|                                |           |          | n.com/en_u |
|                                |           |          | s/AmazonS3 |
|                                |           |          | /latest/de |
|                                |           |          | v/UsingBuc |
|                                |           |          | ket.html>` |
|                                |           |          | __         |
|                                |           |          | name for   |
|                                |           |          | backups    |
+--------------------------------+-----------+----------+------------+
| storages.s3.region             | string    | ``us-eas | The `AWS   |
|                                |           | t-1``    | region <ht |
|                                |           |          | tps://docs |
|                                |           |          | .aws.amazo |
|                                |           |          | n.com/en_u |
|                                |           |          | s/general/ |
|                                |           |          | latest/gr/ |
|                                |           |          | rande.html |
|                                |           |          | >`__       |
|                                |           |          | to use.    |
|                                |           |          | Please     |
|                                |           |          | note       |
|                                |           |          | **this     |
|                                |           |          | option is  |
|                                |           |          | mandatory* |
|                                |           |          | *          |
|                                |           |          | not only   |
|                                |           |          | for Amazon |
|                                |           |          | S3, but    |
|                                |           |          | for all    |
|                                |           |          | S3-compati |
|                                |           |          | ble        |
|                                |           |          | storages.  |
+--------------------------------+-----------+----------+------------+
| storages.s3.endpointUrl        | string    |          | The        |
|                                |           |          | endpoint   |
|                                |           |          | URL of the |
|                                |           |          | S3-compati |
|                                |           |          | ble        |
|                                |           |          | storage to |
|                                |           |          | be used    |
|                                |           |          | (not       |
|                                |           |          | needed for |
|                                |           |          | the        |
|                                |           |          | original   |
|                                |           |          | Amazon S3  |
|                                |           |          | cloud)     |
+--------------------------------+-----------+----------+------------+
| storages.persistentVolumeClaim | string    | ``standa | Set the    |
| .storageClassName              |           | rd``     | `Kubernete |
|                                |           |          | s          |
|                                |           |          | Storage    |
|                                |           |          | Class <htt |
|                                |           |          | ps://kuber |
|                                |           |          | netes.io/d |
|                                |           |          | ocs/concep |
|                                |           |          | ts/storage |
|                                |           |          | /storage-c |
|                                |           |          | lasses/>`_ |
|                                |           |          | _          |
|                                |           |          | to use     |
|                                |           |          | with the   |
|                                |           |          | PXC        |
|                                |           |          | backups    |
|                                |           |          | `Persisten |
|                                |           |          | t          |
|                                |           |          | Volume     |
|                                |           |          | Claim <htt |
|                                |           |          | ps://kuber |
|                                |           |          | netes.io/d |
|                                |           |          | ocs/concep |
|                                |           |          | ts/storage |
|                                |           |          | /persisten |
|                                |           |          | t-volumes/ |
|                                |           |          | #persisten |
|                                |           |          | tvolumecla |
|                                |           |          | ims>`__    |
|                                |           |          | for the    |
|                                |           |          | ``filesyst |
|                                |           |          | em``       |
|                                |           |          | storage    |
|                                |           |          | type       |
+--------------------------------+-----------+----------+------------+
| storages.persistentVolumeClaim | array     | [“ReadWr | The        |
| .accessModes                   |           | iteOnce” | `Kubernete |
|                                |           | ]        | s          |
|                                |           |          | Persistent |
|                                |           |          | Volume     |
|                                |           |          | access     |
|                                |           |          | modes <htt |
|                                |           |          | ps://kuber |
|                                |           |          | netes.io/d |
|                                |           |          | ocs/concep |
|                                |           |          | ts/storage |
|                                |           |          | /persisten |
|                                |           |          | t-volumes/ |
|                                |           |          | #access-mo |
|                                |           |          | des>`__    |
+--------------------------------+-----------+----------+------------+
| schedule.name                  | string    | ``sat-ni | The backup |
|                                |           | ght-back | name       |
|                                |           | up``     |            |
+--------------------------------+-----------+----------+------------+
| schedule.schedule              | string    | ``0 0 *  | Scheduled  |
|                                |           | * 6``    | time to    |
|                                |           |          | make a     |
|                                |           |          | backup,    |
|                                |           |          | specified  |
|                                |           |          | in the     |
|                                |           |          | `crontab   |
|                                |           |          | format <ht |
|                                |           |          | tps://en.w |
|                                |           |          | ikipedia.o |
|                                |           |          | rg/wiki/Cr |
|                                |           |          | on>`__     |
+--------------------------------+-----------+----------+------------+
| schedule.storageName           | string    | ``st-us- | Name of    |
|                                |           | west``   | the        |
|                                |           |          | storage    |
|                                |           |          | for        |
|                                |           |          | backups,   |
|                                |           |          | configured |
|                                |           |          | in the     |
|                                |           |          | ``storages |
|                                |           |          | ``         |
|                                |           |          | or         |
|                                |           |          | ``fs-pvc`` |
|                                |           |          | subsection |
+--------------------------------+-----------+----------+------------+
| schedule.keep                  | int       | ``3``    | Number of  |
|                                |           |          | backups to |
|                                |           |          | store      |
+--------------------------------+-----------+----------+------------+
