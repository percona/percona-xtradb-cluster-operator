.. _operator.custom-resource-options:

`Custom Resource options <operator.html#operator-custom-resource-options>`_
===============================================================================

Percona XtraDB Cluster managed by the Operator configured via the spec section
of the `deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/main/deploy/cr.yaml>`__
file.

The metadata part of this file contains the following keys:

* ``name`` (``my-cluster-name`` by default) sets the name of your Percona
  XtraDB Cluster; it should include only `URL-compatible characters <https://datatracker.ietf.org/doc/html/rfc3986#section-2.3>`_,
  not exceed 22 characters, start with an alphabetic character, and end with an
  alphanumeric character;
* .. _finalizers-order:

  ``finalizers.delete-pods-in-order`` if present, activates the `Finalizer <https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#finalizers>`_ which controls the proper Pods deletion order in case of the cluster deletion event (on by default).
* .. _finalizers-pxc:

  ``finalizers.delete-pxc-pvc``, ``delete-proxysql-pvc`` if present, activates the `Finalizer <https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#finalizers>`_ which deletes `Persistent Volume Claims <https://kubernetes.io/docs/concepts/storage/persistent-volumes/>`_ for Percona XtraDB Cluster Pods after the cluster deletion event (off by default).
* .. _finalizers-proxysql:

  ``delete-proxysql-pvc`` if present, activates the `Finalizer <https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#finalizers>`_ which deletes `Persistent Volume Claim <https://kubernetes.io/docs/concepts/storage/persistent-volumes/>`_ for ProxySQL Pod after the cluster deletion event (off by default).

The spec part of the `deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/main/deploy/cr.yaml>`__ file contains the following sections:

.. tabularcolumns:: |p{40mm}|p{10mm}|p{49mm}|p{47mm}|

.. list-table::
   :widths: 25 9 31 35
   :header-rows: 1

   * - Key
     - Value type
     - Default
     - Description

   * - upgradeOptions
     - :ref:`subdoc<operator.upgradeoptions-section>`
     -
     - Percona XtraDB Cluster upgrade options section

   * - pxc
     - :ref:`subdoc<operator.pxc-section>`
     -
     - Percona XtraDB Cluster general section

   * - haproxy
     - :ref:`subdoc<operator.haproxy-section>`
     -
     - HAProxy section

   * - proxysql
     - :ref:`subdoc<operator.proxysql-section>`
     -
     - ProxySQL section

   * - pmm
     - :ref:`subdoc<operator.pmm-section>`
     -
     - Percona Monitoring and Management section

   * - backup
     - :ref:`subdoc<operator.backup-section>`
     -
     - Percona XtraDB Cluster backups section

   * - allowUnsafeConfigurations
     - boolean
     - ``false``
     - Prevents users from configuring a cluster with unsafe parameters such as starting the cluster with the number of Percona XtdaDB Cluster instances which is less than 3, more than 5, or is an even number, with less than 2 ProxySQL or HAProxy Pods, or without TLS/SSL certificates (if ``false``, unsafe parameters will be automatically changed to safe defaults)

   * - enableCRValidationWebhook
     - boolean
     - ``true``
     - Enables or disables schema validation before applying ``cr.yaml`` file (works only in :ref:`cluster-wide mode<install-clusterwide>` due to
       :ref:`access restrictions<faq-validation>`).

   * - pause
     - boolean
     - ``false``
     - Pause/resume: setting it to ``true`` gracefully stops the cluster, and
       setting it to ``false`` after shut down starts the cluster back.

   * - secretsName
     - string
     - ``my-cluster-secrets``
     - A name for :ref:`users secrets<users>`

   * - crVersion
     - string
     - ``{{{release}}}``
     - Version of the Operator the Custom Resource belongs to

   * - vaultSecretName
     - string
     - ``keyring-secret-vault``
     - A secret for the `HashiCorp Vault <https://www.vaultproject.io/>`_ to carry on :ref:`encryption`

   * - sslSecretName
     - string
     - ``my-cluster-ssl``
     - A secret with TLS certificate generated for *external* communications, see :ref:`tls` for details

   * - sslInternalSecretName
     - string
     - ``my-cluster-ssl-internal``
     - A secret with TLS certificate generated for *internal* communications, see :ref:`tls` for details

   * - logCollectorSecretName
     - string
     - ``my-log-collector-secrets``
     - A secret for the `Fluent Bit Log Collector <https://fluentbit.io>`_

   * - initImage
     - string
     - ``percona/percona-xtradb-cluster-operator:{{{release}}}``
     - An alternative image for the initial Operator installation

   * - tls.SANs
     - subdoc 
     -
     - Additional domains (SAN) to be added to the TLS certificate within the
       extended cert-manager configuration 

   * - issuerConf
     - :ref:`subdoc<operator.issuerconf-section>`
     - 
     - Extended cert-manager configuration section

   * - updateStrategy
     - string
     - ``SmartUpdate``
     - A strategy the Operator uses for :ref:`upgrades<operator-update>`

.. _operator.issuerconf-section:

`Extended cert-manager Configuration Section <operator.html#operator-issuerconf-section>`_
------------------------------------------------------------------------------------------

The ``issuerConf`` section in the `deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/main/deploy/cr.yaml>`__ file contains various configuration options for additional customization of the `TLS certificates issuer <tls.certs.certmanager>`_.

.. tabularcolumns:: |p{2cm}|p{13.6cm}|

+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _issuerconf-name:                                                                      |
|                 |                                                                                           |
| **Key**         | `issuerConf.name <operator.html#issuerconf-name>`_                                        |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``special-selfsigned-issuer``                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | A `cert-manager issuer name <https://cert-manager.io/docs/concepts/issuer/>`__            |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _issuerconf-kind:                                                                      |
|                 |                                                                                           |
| **Key**         | `issuerConf.kind <operator.html#issuerconf-kind>`_                                        |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``ClusterIssuer``                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | A `cert-manager issuer type <https://cert-manager.io/docs/configuration/>`__              |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _issuerconf-group:                                                                     |
|                 |                                                                                           |
| **Key**         | `issuerConf.group <operator.html#issuerconf-group>`_                                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``cert-manager.io``                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | A `cert-manager issuer group <https://cert-manager.io/docs/configuration/>`__. Should be  |
|                 | ``cert-manager.io`` for built-in cert-manager certificate issuers                         |
+-----------------+-------------------------------------------------------------------------------------------+

.. _operator.upgradeoptions-section:

`Upgrade Options Section <operator.html#operator-upgradeoptions-section>`_
--------------------------------------------------------------------------------

The ``upgradeOptions`` section in the `deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/main/deploy/cr.yaml>`__ file contains various configuration options to control Percona XtraDB Cluster upgrades.

.. tabularcolumns:: |p{2cm}|p{13.6cm}|

+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _upgradeoptions-versionserviceendpoint:                                                |
|                 |                                                                                           |
| **Key**         | `upgradeOptions.versionServiceEndpoint                                                    |
|                 | <operator.html#upgradeoptions-versionserviceendpoint>`_                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``https://check.percona.com``                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The Version Service URL used to check versions compatibility for upgrade                  |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _upgradeoptions-apply:                                                                 |
|                 |                                                                                           |
| **Key**         | `upgradeOptions.apply <operator.html#upgradeoptions-apply>`_                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``Disabled``                                                                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Specifies how :ref:`updates are processed<operator-update-smartupdates>` by the Operator. |
|                 | ``Never`` or ``Disabled`` will completely disable automatic upgrades, otherwise it can be |
|                 | set to ``Latest`` or ``Recommended`` or to a specific version string of Percona XtraDB    |
|                 | Cluster (e.g. ``8.0.19-10.1``) that is wished to be version-locked (so that the user can  |
|                 | control the version running, but use automatic upgrades to move between them).            |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _upgradeoptions-schedule:                                                              |
|                 |                                                                                           |
| **Key**         | `upgradeOptions.schedule <operator.html#upgradeoptions-schedule>`_                        |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``0 2 * * *``                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Scheduled time to check for updates, specified in the                                     |
|                 | `crontab format <https://en.wikipedia.org/wiki/Cron>`_                                    |
+-----------------+-------------------------------------------------------------------------------------------+

.. _operator.pxc-section:

`PXC Section <operator.html#operator-pxc-section>`_
--------------------------------------------------------------------------------

The ``pxc`` section in the `deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/main/deploy/cr.yaml>`__ file contains general
configuration options for the Percona XtraDB Cluster.

.. tabularcolumns:: |p{2cm}|p{13.6cm}|

+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-size:                                                                             |
|                 |                                                                                           |
| **Key**         | `pxc.size <operator.html#pxc-size>`_                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``3``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The size of the Percona XtraDB cluster must be 3 or 5 for                                 |
|                 | `High Availability <https://www.percona.com/doc/percona-xtradb-cluster/5.7/intro.html>`_. |
|                 | other values are allowed if the ``spec.allowUnsafeConfigurations`` key is set to true.    |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-image:                                                                            |
|                 |                                                                                           |
| **Key**         | `pxc.image <operator.html#pxc-image>`_                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``percona/percona-xtradb-cluster:{{{pxc80recommended}}}``                                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The Docker image of the Percona cluster used (actual image names for Percona XtraDB       |
|                 | Cluster 8.0 and Percona XtraDB Cluster 5.7 can be found                                   |
|                 | :ref:`in the list of certified images<custom-registry-images>`)                           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-autorecovery:                                                                     |
|                 |                                                                                           |
| **Key**         | `pxc.autoRecovery <operator.html#pxc-autorecovery>`_                                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | boolean                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``true``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Turns :ref:`recovery-auto` on or off                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-expose-enabled:                                                                   |
|                 |                                                                                           |
| **Key**         | `pxc.expose.enabled <operator.html#pxc-expose-enabled>`_                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | boolean                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``true``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Enable or disable exposing Percona XtraDB Cluster nodes with dedicated IP addresses       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-expose-type:                                                                      |
|                 |                                                                                           |
| **Key**         | `pxc.expose.type <operator.html#pxc-expose-type>`_                                        |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``LoadBalancer``                                                                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes Service Type                                                              |
|                 | <https://kubernetes.io/docs/concepts/services-networking/service/                         |
|                 | #publishing-services-service-types>`_ used for xposure                                    |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-expose-trafficpolicy:                                                             |
|                 |                                                                                           |
| **Key**         | `pxc.expose.trafficPolicy <operator.html#pxc-expose-trafficpolicy>`_                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``Local``                                                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Specifies whether Service should `route external traffic to cluster-wide or node-local    |
|                 | endpoints <https://kubernetes.io/docs/tasks/access-application-cluster/                   |
|                 | create-external-load-balancer/#preserving-the-client-source-ip>`_ (it can influence the   |
|                 | load balancing effectiveness)                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-expose-loadbalancersourceranges:                                                  |
|                 |                                                                                           |
| **Key**         | `pxc.expose.loadBalancerSourceRanges                                                      |
|                 | <operator.html#pxc-expose-loadbalancersourceranges>`_                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``10.0.0.0/8``                                                                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The range of client IP addresses from which the load balancer should be reachable         |
|                 | (if not set, there is no limitations)                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-expose-annotations:                                                               |
|                 |                                                                                           |
| **Key**         | `pxc.expose.annotations <operator.html#pxc-expose-annotations>`_                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``networking.gke.io/load-balancer-type: "Internal"``                                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes annotations                                                               |
|                 | <https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/>`_        |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-replicationchannels-name:                                                         |
|                 |                                                                                           |
| **Key**         | `pxc.replicationChannels.name <operator.html#pxc-replicationchannels-name>`_              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``pxc1_to_pxc2``                                                                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Name of the replication channel for :ref:`cross-site replication<operator-replication>`   |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-replicationchannels-issource:                                                     |
|                 |                                                                                           |
| **Key**         | `pxc.replicationChannels.isSource <operator.html#pxc-replicationchannels-issource>`_      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | boolean                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``false``                                                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Should the cluster act as Source (``true``) or Replica (``false``)                        |
|                 | in :ref:`cross-site replication<operator-replication>`                                    |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-replicationchannels-configuration-sourceretrycount:                               |
|                 |                                                                                           |
| **Key**         | `pxc.replicationChannels.configuration.sourceRetryCount                                   |
|                 | <operator.html#pxc-replicationchannels-configuration-sourceretrycount>`_                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``3``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Number of retries Replica should do when the existing connection source fails             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-replicationchannels-configuration-sourceconnectretry:                             |
|                 |                                                                                           |
| **Key**         | `pxc.replicationChannels.configuration.sourceConnectRetry                                 |
|                 | <operator.html#pxc-replicationchannels-configuration-sourceconnectretry>`_                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``60``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The interval between reconnection attempts in seconds to be used by Replica when the      |
|                 | the existing connection source fails                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-replicationchannels-sourceslist-host:                                             |
|                 |                                                                                           |
| **Key**         | `pxc.replicationChannels.sourcesList.host                                                 |
|                 | <operator.html#pxc-replicationchannels-sourceslist-host>`_                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``10.95.251.101``                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | For the :ref:`cross-site replication<operator-replication>` Replica cluster, this key     |
|                 | should contain the hostname or IP address of the Source cluster                           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-replicationchannels-sourceslist-port:                                             |
|                 |                                                                                           |
| **Key**         | `pxc.replicationChannels.sourcesList.port                                                 |
|                 | <operator.html#pxc-replicationchannels-sourceslist-port>`_                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``3306``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | For the :ref:`cross-site replication<operator-replication>` Replica cluster, this key     |
|                 | should contain the Source port number                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-replicationchannels-sourceslist-weight:                                           |
|                 |                                                                                           |
| **Key**         | `pxc.replicationChannels.sourcesList.weight                                               |
|                 | <operator.html#pxc-replicationchannels-sourceslist-weight>`_                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``100``                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | For the :ref:`cross-site replication<operator-replication>` Replica cluster, this key     |
|                 | should contain the Source cluster weight (varies from ``1`` to ``100``, the cluster with  |
|                 | the higher number will be selected as the replication source first)                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-readinessdelaysec:                                                                |
|                 |                                                                                           |
| **Key**         | `pxc.readinessDelaySec <operator.html#pxc-readinessdelaysec>`_                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``15``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Adds a delay before a run check to verify the application is ready to process traffic     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-livenessdelaysec:                                                                 |
|                 |                                                                                           |
| **Key**         | `pxc.livenessDelaySec <operator.html#pxc-livenessdelaysec>`_                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``300``                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Adds a delay before the run check ensures the application is healthy and capable of       |
|                 | processing requests                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-configuration:                                                                    |
|                 |                                                                                           |
| **Key**         | `pxc.configuration <operator.html#pxc-configuration>`_                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``|``                                                                                     |
|                 |                                                                                           |
|                 | ``[mysqld]``                                                                              |
|                 |                                                                                           |
|                 | ``wsrep_debug=ON``                                                                        |
|                 |                                                                                           |
|                 | ``wsrep-provider_options=gcache.size=1G;gcache.recover=yes``                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The ``my.cnf`` file options to be passed to Percona XtraDB cluster nodes                  |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-imagepullsecrets-name:                                                            |
|                 |                                                                                           |
| **Key**         | `pxc.imagePullSecrets.name <operator.html#pxc-imagepullsecrets-name>`_                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``private-registry-credentials``                                                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes ImagePullSecret                                                           |
|                 | <https://kubernetes.io/docs/concepts/configuration/secret/#using-imagepullsecrets>`_      |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-priorityclassname:                                                                |
|                 |                                                                                           |
| **Key**         | `pxc.priorityClassName <operator.html#pxc-priorityclassname>`_                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``high-priority``                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes Pod priority class                                                        |
|                 | <https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/               |
|                 | #priorityclass>`_                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-schedulername:                                                                    |
|                 |                                                                                           |
| **Key**         | `pxc.schedulerName <operator.html#pxc-schedulername>`_                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``mycustom-scheduler``                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes Scheduler                                                                 |
|                 | <https://kubernetes.io/docs/tasks/administer-cluster/configure-multiple-schedulers>`_     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-annotations:                                                                      |
|                 |                                                                                           |
| **Key**         | `pxc.annotations <operator.html#pxc-annotations>`_                                        |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | label                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``iam.amazonaws.com/role: role-arn``                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes annotations                                                               |
|                 | <https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/>`_        |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-labels:                                                                           |
|                 |                                                                                           |
| **Key**         | `pxc.labels <operator.html#pxc-labels>`_                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | label                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``rack: rack-22``                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Labels are key-value pairs attached to objects                                           |
|                 | <https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/>`_             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-readinessprobes-initialdelayseconds:                                              |
|                 |                                                                                           |
| **Key**         | `pxc.readinessProbes.initialDelaySeconds                                                  |
|                 | <operator.html#pxc-readinessprobes-initialdelayseconds>`_                                 |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``15``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Number of seconds to wait before performing the first `readiness probe                    |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/                                |
|                 | configure-liveness-readiness-startup-probes/>`_                                           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-readinessprobes-timeoutseconds:                                                   |
|                 |                                                                                           |
| **Key**         | `pxc.readinessProbes.timeoutSeconds                                                       |
|                 | <operator.html#pxc-readinessprobes-timeoutseconds>`_                                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``15``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Number of seconds after the container has started before `readiness probes                |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/                                |
|                 | configure-liveness-readiness-startup-probes/>`_ are initiated                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-readinessprobes-periodseconds:                                                    |
|                 |                                                                                           |
| **Key**         | `pxc.readinessProbes.periodSeconds                                                        |
|                 | <operator.html#pxc-readinessprobes-periodseconds>`_                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``30``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | How often (in seconds) to perform the `readiness probe                                    |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/                                |
|                 | configure-liveness-readiness-startup-probes/>`_                                           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-readinessprobes-successthreshold:                                                 |
|                 |                                                                                           |
| **Key**         | `pxc.readinessProbes.successThreshold                                                     |
|                 | <operator.html#pxc-readinessprobes-successthreshold>`_                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Minimum consecutive successes for the `readiness probe                                    |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/                                |
|                 | configure-liveness-readiness-startup-probes/>`_ to be considered successful after having  |
|                 | failed                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-readinessprobes-failurethreshold:                                                 |
|                 |                                                                                           |
| **Key**         | `pxc.readinessProbes.failureThreshold                                                     |
|                 | <operator.html#pxc-readinessprobes-failurethreshold>`_                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``5``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | When the `readiness probe <https://kubernetes.io/docs/tasks/configure-pod-container/      |
|                 | configure-liveness-readiness-startup-probes/>`_ fails, Kubernetes will try this number of |
|                 | times before marking the Pod Unready                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-livenessprobes-initialdelayseconds:                                               |
|                 |                                                                                           |
| **Key**         | `pxc.livenessProbes.initialDelaySeconds                                                   |
|                 | <operator.html#pxc-livenessprobes-initialdelayseconds>`_                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``300``                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Number of seconds to wait before performing the first `liveness probe                     |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/                                |
|                 | configure-liveness-readiness-startup-probes/>`_                                           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-livenessprobes-timeoutseconds:                                                    |
|                 |                                                                                           |
| **Key**         | `pxc.livenessProbes.timeoutSeconds                                                        |
|                 | <operator.html#pxc-livenessprobes-timeoutseconds>`_                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``5``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Number of seconds after the container has started before `liveness probes                 |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/                                |
|                 | configure-liveness-readiness-startup-probes/>`_ are initiated                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-livenessprobes-periodseconds:                                                     |
|                 |                                                                                           |
| **Key**         | `pxc.livenessProbes.periodSeconds                                                         |
|                 | <operator.html#pxc-livenessprobes-periodseconds>`_                                        |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``10``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | How often (in seconds) to perform the `liveness probe                                     |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/                                |
|                 | configure-liveness-readiness-startup-probes/>`_                                           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-livenessprobes-successthreshold:                                                  |
|                 |                                                                                           |
| **Key**         | `pxc.livenessProbes.successThreshold                                                      |
|                 | <operator.html#pxc-livenessprobes-successthreshold>`_                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Minimum consecutive successes for the `liveness probe                                     |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/                                |
|                 | configure-liveness-readiness-startup-probes/>`_ to be considered successful after having  |
|                 | failed                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-livenessprobes-failurethreshold:                                                  |
|                 |                                                                                           |
| **Key**         | `pxc.livenessProbes.failureThreshold                                                      |
|                 | <operator.html#pxc-livenessprobes-failurethreshold>`_                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``3``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | When the `liveness probe <https://kubernetes.io/docs/tasks/configure-pod-container/       |
|                 | configure-liveness-readiness-startup-probes/>`_ fails, Kubernetes will try this number of |
|                 | times before restarting the container                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-envvarssecret:                                                                    |
|                 |                                                                                           |
| **Key**         | `pxc.envVarsSecret <operator.html#pxc-envvarssecret>`_                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``my-env-var-secrets``                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | A secret with environment variables, see :ref:`faq-env` for details                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-resources-requests-memory:                                                        |
|                 |                                                                                           |
| **Key**         | `pxc.resources.requests.memory <operator.html#pxc-resources-requests-memory>`_            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1G``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes memory requests                                                           |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_                                     |
|                 | for a Percona XtraDB Cluster container                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-resources-requests-cpu:                                                           |
|                 |                                                                                           |
| **Key**         | `pxc.resources.requests.cpu <operator.html#pxc-resources-requests-cpu>`_                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``600m``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes CPU requests                                                                  |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for a Percona XtraDB Cluster        |
|                 | container                                                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-resources-requests-ephemeral-storage:                                             |
|                 |                                                                                           |
| **Key**         | `pxc.resources.requests.ephemeral-storage                                                 |
|                 | <operator.html#pxc-resources-requests-ephemeral-storage>`_                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1G``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Kubernetes `Ephemeral Storage                                                             |
|                 | <https://kubernetes.io/docs/concepts/storage/ephemeral-volumes/>`_ `requests              |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for a Percona XtraDB Cluster        |
|                 | container                                                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-resources-limits-memory:                                                          |
|                 |                                                                                           |
| **Key**         | `pxc.resources.limits.memory <operator.html#pxc-resources-limits-memory>`_                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1G``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes memory limits                                                                 |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for a Percona XtraDB Cluster        |
|                 | container                                                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-resources-limits-cpu:                                                             |
|                 |                                                                                           |
| **Key**         | `pxc.resources.limits.cpu <operator.html#pxc-resources-limits-cpu>`_                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes CPU limits                                                                    |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for a Percona XtraDB Cluster        |
|                 | container                                                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-resources-limits-ephemeral-storage:                                               |
|                 |                                                                                           |
| **Key**         | `pxc.resources.limits.ephemeral-storage                                                   |
|                 | <operator.html#pxc-resources-limits-ephemeral-storage>`_                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1G``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Kubernetes `Ephemeral Storage                                                             |
|                 | <https://kubernetes.io/docs/concepts/storage/ephemeral-volumes/>`_ `limits                |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for a Percona XtraDB Cluster        |
|                 | container                                                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-nodeselector:                                                                     |
|                 |                                                                                           |
| **Key**         | `pxc.nodeSelector <operator.html#pxc-nodeselector>`_                                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | label                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``disktype: ssd``                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes nodeSelector                                                                  |
|                 | <https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector>`_       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-affinity-topologykey:                                                             |
|                 |                                                                                           |
| **Key**         | `pxc.affinity.topologyKey <operator.html#pxc-affinity-topologykey>`_                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``kubernetes.io/hostname``                                                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The Operator `topology key                                                                |
|                 | <https://kubernetes.io/docs/concepts/configuration/assign-pod-node/                       |
|                 | #affinity-and-anti-affinity>`_ node anti-affinity constraint                              |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-affinity-advanced:                                                                |
|                 |                                                                                           |
| **Key**         | `pxc.affinity.advanced <operator.html#pxc-affinity-advanced>`_                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | subdoc                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     |                                                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | In cases where the Pods require complex tuning the `advanced` option turns off the        |
|                 | ``topologyKey`` effect. This setting allows the standard Kubernetes affinity constraints  |
|                 | of any complexity to be used                                                              |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-tolerations:                                                                      |
|                 |                                                                                           |
| **Key**         | `pxc.tolerations <operator.html#pxc-tolerations>`_                                        |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | subdoc                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``node.alpha.kubernetes.io/unreachable``                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes Pod tolerations                                                               |
|                 | <https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/>`_               |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-poddisruptionbudget-maxunavailable:                                               |
|                 |                                                                                           |
| **Key**         | `pxc.podDisruptionBudget.maxUnavailable                                                   |
|                 | <operator.html#pxc-poddisruptionbudget-maxunavailable>`_                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes podDisruptionBudget                                                       |
|                 | <https://kubernetes.io/docs/tasks/run-application/configure-pdb/                          |
|                 | #specifying-a-poddisruptionbudget>`_ specifies the number of Pods from the set            |
|                 | unavailable after the eviction                                                            |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-poddisruptionbudget-minavailable:                                                 |
|                 |                                                                                           |
| **Key**         | `pxc.podDisruptionBudget.minAvailable                                                     |
|                 | <operator.html#pxc-poddisruptionbudget-minavailable>`_                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``0``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes podDisruptionBudget                                                       |
|                 | <https://kubernetes.io/docs/tasks/run-application/configure-pdb/                          |
|                 | #specifying-a-poddisruptionbudget>`_ Pods that must be available after an eviction        |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-volumespec-emptydir:                                                              |
|                 |                                                                                           |
| **Key**         | `pxc.volumeSpec.emptyDir <operator.html#pxc-volumespec-emptydir>`_                        |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``{}``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes emptyDir volume                                                           |
|                 | <https://kubernetes.io/docs/concepts/storage/volumes/#emptydir>`_ The directory created   |
|                 | on a node and accessible to the Percona XtraDB Cluster Pod containers                     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-volumespec-hostpath-path:                                                         |
|                 |                                                                                           |
| **Key**         | `pxc.volumeSpec.hostPath.path <operator.html#pxc-volumespec-hostpath-path>`_              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``/data``                                                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes hostPath <https://kubernetes.io/docs/concepts/storage/volumes/#hostpath>`_    |
|                 | The volume that mounts a directory from the host node's filesystem into your Pod. The     |
|                 | path property is required                                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-volumespec-hostpath-type:                                                         |
|                 |                                                                                           |
| **Key**         | `pxc.volumeSpec.hostPath.type <operator.html#pxc-volumespec-hostpath-type>`_              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``Directory``                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes hostPath <https://kubernetes.io/docs/concepts/storage/volumes/            |
|                 | #hostpath>`_. An optional property for the hostPath                                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-volumespec-persistentvolumeclaim-storageclassname:                                |
|                 |                                                                                           |
| **Key**         | `pxc.volumeSpec.persistentVolumeClaim.storageClassName                                    |
|                 | <operator.html#pxc-volumespec-persistentvolumeclaim-storageclassname>`_                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``standard``                                                                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Set the `Kubernetes storage class                                                         |
|                 | <https://kubernetes.io/docs/concepts/storage/storage-classes/>`_ to use with the Percona  |
|                 | XtraDB Cluster `PersistentVolumeClaim                                                     |
|                 | <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims>`_|
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-volumespec-persistentvolumeclaim-accessmodes:                                     |
|                 |                                                                                           |
| **Key**         | `pxc.volumeSpec.persistentVolumeClaim.accessModes                                         |
|                 | <operator.html#pxc-volumespec-persistentvolumeclaim-accessmodes>`_                        |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | array                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``[ReadWriteOnce]``                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes PersistentVolumeClaim                                                     |
|                 | <https://kubernetes.io/docs/concepts/storage/persistent-volumes/                          |
|                 | #persistentvolumeclaims>`_ access modes for the Percona XtraDB cluster                    |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-volumespec-resources-requests-storage:                                            |
|                 |                                                                                           |
| **Key**         | `pxc.volumeSpec.resources.requests.storage                                                |
|                 | <operator.html#pxc-volumespec-resources-requests-storage>`_                               |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``6Gi``                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes PersistentVolumeClaim                                                     |
|                 | <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#                         |
|                 | persistentvolumeclaims>`_ size for the Percona XtraDB cluster                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-graceperiod:                                                                      |
|                 |                                                                                           |
| **Key**         | `pxc.gracePeriod <operator.html#pxc-graceperiod>`_                                        |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``600``                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes grace period when terminating a Pod                                       |
|                 | <https://kubernetes.io/docs/concepts/workloads/pods/pod/#termination-of-pods>`_           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-containersecuritycontext:                                                         |
|                 |                                                                                           |
| **Key**         | `pxc.containerSecurityContext <operator.html#pxc-containersecuritycontext>`_              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | subdoc                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``privileged: true``                                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | A custom `Kubernetes Security Context for a Container                                     |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/security-context/>`_ to be used |
|                 | instead of the default one                                                                |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-podsecuritycontext:                                                               |
|                 |                                                                                           |
| **Key**         | `pxc.podSecurityContext <operator.html#pxc-podsecuritycontext>`_                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | subdoc                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``fsGroup: 1001``                                                                         |
|                 |                                                                                           |
|                 | ``supplementalGroups: [1001, 1002, 1003]``                                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | A custom `Kubernetes Security Context for a Pod                                           |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/security-context/>`_ to be used |
|                 | instead of the default one                                                                |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-serviceaccountname:                                                               |
|                 |                                                                                           |
| **Key**         | `pxc.serviceAccountName <operator.html#pxc-serviceaccountname>`_                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``percona-xtradb-cluster-operator-workload``                                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes Service Account                                                           |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/>`_   |
|                 | for Percona XtraDB Cluster Pods                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-imagepullpolicy:                                                                  |
|                 |                                                                                           |
| **Key**         | `pxc.imagePullPolicy <operator.html#_pxc-imagepullpolicy>`_                               |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``Always``                                                                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `policy used to update images <https://kubernetes.io/docs/concepts/containers/images/ |
|                 | #updating-images>`_                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-runtimeclassname:                                                                 |
|                 |                                                                                           |
| **Key**         | `pxc.runtimeClassName                                                                     |
|                 | <operator.html#pxc-runtimeclassname>`_                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``image-rc``                                                                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Name of the `Kubernetes Runtime Class                                                     |
|                 | <https://kubernetes.io/docs/concepts/containers/runtime-class/>`_                         |
|                 | for Percona XtraDB Cluster Pods                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-sidecars-image:                                                                   |
|                 |                                                                                           |
| **Key**         | `pxc.sidecars.image                                                                       |
|                 | <operator.html#pxc-sidecars-image>`_                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``busybox``                                                                               |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Image for the                                                                             |
|                 | :ref:`custom sidecar container<faq-sidecar>`                                              |
|                 | for Percona XtraDB Cluster Pods                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-sidecars-command:                                                                 |
|                 |                                                                                           |
| **Key**         | `pxc.sidecars.command                                                                     |
|                 | <operator.html#pxc-sidecars-command>`_                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | array                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``["/bin/sh"]``                                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Command for the                                                                           |
|                 | :ref:`custom sidecar container<faq-sidecar>`                                              |
|                 | for Percona XtraDB Cluster Pods                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-sidecars-args:                                                                    |
|                 |                                                                                           |
| **Key**         | `pxc.sidecars.args                                                                        |
|                 | <operator.html#pxc-sidecars-args>`_                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | array                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``["-c", "while true; do trap 'exit 0' SIGINT SIGTERM SIGQUIT SIGKILL; done;"]``          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Command arguments for the                                                                 |
|                 | :ref:`custom sidecar container<faq-sidecar>`                                              |
|                 | for Percona XtraDB Cluster Pods                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-sidecars-name:                                                                    |
|                 |                                                                                           |
| **Key**         | `pxc.sidecars.name                                                                        |
|                 | <operator.html#pxc-sidecars-name>`_                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``my-sidecar-1``                                                                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Name of the                                                                               |
|                 | :ref:`custom sidecar container<faq-sidecar>`                                              |
|                 | for Percona XtraDB Cluster Pods                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-sidecars-resources-requests-memory:                                               |
|                 |                                                                                           |
| **Key**         | `pxc.sidecars.resources.requests.memory <operator.html#                                   |
|                 | pxc-sidecars-resources-requests-memory>`_                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1G``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes memory requests                                                           |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_                                     |
|                 | for a Percona XtraDB Cluster sidecar container                                            |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-sidecars-resources-requests-cpu:                                                  |
|                 |                                                                                           |
| **Key**         | `pxc.sidecars.resources.requests.cpu <operator.html#pxc-sidecars-resources-requests-cpu>`_|
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``500m``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes CPU requests                                                                  |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for a Percona XtraDB Cluster        |
|                 | sidecar container                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-sidecars-resources-limits-memory:                                                 |
|                 |                                                                                           |
| **Key**         | `pxc.sidecars.resources.limits.memory <operator.html#                                     |
|                 | pxc-sidecars-resources-limits-memory>`_                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``2G``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes memory limits                                                                 |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for a Percona XtraDB Cluster        |
|                 | sidecar container                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pxc-sidecars-resources-limits-cpu:                                                    |
|                 |                                                                                           |
| **Key**         | `pxc.sidecars.resources.limits.cpu <operator.html#pxc-sidecars-resources-limits-cpu>`_    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``600m``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes CPU limits                                                                    |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for a Percona XtraDB Cluster        |
|                 | sidecar container                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+

.. _operator.haproxy-section:

`HAProxy Section <operator.html#operator-haproxy-section>`_
--------------------------------------------------------------------------------

The ``haproxy`` section in the `deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/main/deploy/cr.yaml>`__ file contains
configuration options for the HAProxy service.

.. tabularcolumns:: |p{2cm}|p{13.6cm}|

+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-enabled:                                                                      |
|                 |                                                                                           |
| **Key**         | `haproxy.enabled <operator.html#haproxy-enabled>`_                                        |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | boolean                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``true``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Enables or disables `load balancing with HAProxy                                          |
|                 | <https://www.percona.com/doc/percona-xtradb-cluster/8.0/howtos/haproxy.html>`_ `Services  |
|                 | <https://kubernetes.io/docs/concepts/services-networking/service/>`_                      |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-size:                                                                         |
|                 |                                                                                           |
| **Key**         | `haproxy.size <operator.html#haproxy-size>`_                                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``2``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The number of the HAProxy Pods `to provide load balancing                                 |
|                 | <https://www.percona.com/doc/percona-xtradb-cluster/8.0/howtos/haproxy.html>`__. It should|
|                 | be 2 or more unless the ``spec.allowUnsafeConfigurations`` key is set to true.            |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-image:                                                                        |
|                 |                                                                                           |
| **Key**         | `haproxy.image <operator.html#haproxy-image>`_                                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``percona/percona-xtradb-cluster-operator:{{{release}}}-haproxy``                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | HAProxy Docker image to use                                                               |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-replicasserviceenabled:                                                       |
|                 |                                                                                           |
| **Key**         | `haproxy.replicasServiceEnabled <operator.html#haproxy-replicasserviceenabled>`_          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | boolean                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``true``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Enables or disables ``haproxy-replicas`` Service. This Service (on by default) forwards   |
|                 | requests to all Percona XtraDB Cluster instances, and it **should not be used for write** |
|                 | **requests**!                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-imagepullpolicy:                                                              |
|                 |                                                                                           |
| **Key**         | `haproxy.imagePullPolicy <operator.html#haproxy-imagepullpolicy>`_                        |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``Always``                                                                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `policy used to update images <https://kubernetes.io/docs/concepts/containers/images/ |
|                 | #updating-images>`_                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-imagepullsecrets-name:                                                        |
|                 |                                                                                           |
| **Key**         | `haproxy.imagePullSecrets.name <operator.html#haproxy-imagepullsecrets-name>`_            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``private-registry-credentials``                                                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes imagePullSecrets                                                          |
|                 | <https://kubernetes.io/docs/concepts/configuration/secret/#using-imagepullsecrets>`_ for  |
|                 | the HAProxy image                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-readinessdelaysec:                                                            |
|                 |                                                                                           |
| **Key**         | `haproxy.readinessDelaySec <operator.html#haproxy-readinessdelaysec>`_                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``15``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Adds a delay before a run check to verify the application is ready to process traffic     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-livenessdelaysec:                                                             |
|                 |                                                                                           |
| **Key**         | `haproxy.livenessDelaySec <operator.html#haproxy-livenessdelaysec>`_                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``300``                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Adds a delay before the run check ensures the application is healthy and capable of       |
|                 | processing requests                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-configuration:                                                                |
|                 |                                                                                           |
| **Key**         | `haproxy.configuration <operator.html#haproxy-configuration>`_                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     |                                                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The :ref:`custom HAProxy configuration file<haproxy-conf-custom>` contents                |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-annotations:                                                                  |
|                 |                                                                                           |
| **Key**         | `haproxy.annotations <operator.html#haproxy-annotations>`_                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | label                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``iam.amazonaws.com/role: role-arn``                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes annotations                                                               |
|                 | <https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/>`_        |
|                 | metadata                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-labels:                                                                       |
|                 |                                                                                           |
| **Key**         | `haproxy.labels <operator.html#haproxy-labels>`_                                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | label                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``rack: rack-22``                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Labels are key-value pairs attached to objects                                           |
|                 | <https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/>`_             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-readinessprobes-initialdelayseconds:                                          |
|                 |                                                                                           |
| **Key**         | `haproxy.readinessProbes.initialDelaySeconds                                              |
|                 | <operator.html#haproxy-readinessprobes-initialdelayseconds>`_                             |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``15``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Number of seconds to wait before performing the first `readiness probe                    |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/                                |
|                 | configure-liveness-readiness-startup-probes/>`_                                           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-readinessprobes-timeoutseconds:                                               |
|                 |                                                                                           |
| **Key**         | `haproxy.readinessProbes.timeoutSeconds                                                   |
|                 | <operator.html#haproxy-readinessprobes-timeoutseconds>`_                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Number of seconds after the container has started before `readiness probes                |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/                                |
|                 | configure-liveness-readiness-startup-probes/>`_ are initiated                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-readinessprobes-periodseconds:                                                |
|                 |                                                                                           |
| **Key**         | `haproxy.readinessProbes.periodSeconds                                                    |
|                 | <operator.html#haproxy-readinessprobes-periodseconds>`_                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``5``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | How often (in seconds) to perform the `readiness probe                                    |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/                                |
|                 | configure-liveness-readiness-startup-probes/>`_                                           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-readinessprobes-successthreshold:                                             |
|                 |                                                                                           |
| **Key**         | `haproxy.readinessProbes.successThreshold                                                 |
|                 | <operator.html#haproxy-readinessprobes-successthreshold>`_                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Minimum consecutive successes for the `readiness probe                                    |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/                                |
|                 | configure-liveness-readiness-startup-probes/>`_ to be considered successful after having  |
|                 | failed                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-readinessprobes-failurethreshold:                                             |
|                 |                                                                                           |
| **Key**         | `haproxy.readinessProbes.failureThreshold                                                 |
|                 | <operator.html#haproxy-readinessprobes-failurethreshold>`_                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``3``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | When the `readiness probe <https://kubernetes.io/docs/tasks/configure-pod-container/      |
|                 | configure-liveness-readiness-startup-probes/>`_ fails, Kubernetes will try this number of |
|                 | times before marking the Pod Unready                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-servicetype:                                                                  |
|                 |                                                                                           |
| **Key**         | `haproxy.serviceType <operator.html#haproxy-servicetype>`_                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``ClusterIP``                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Specifies the type of `Kubernetes Service                                                 |
|                 | <https://kubernetes.io/docs/concepts/services-networking/service/                         |
|                 | #publishing-services-service-types>`_ to be used for HAProxy                              |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-externaltrafficpolicy:                                                        |
|                 |                                                                                           |
| **Key**         | `haproxy.externalTrafficPolicy <operator.html#haproxy-externaltrafficpolicy>`_            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``Cluster``                                                                               |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Specifies whether Service for HAProxy should `route external traffic to cluster-wide or   |
|                 | to node-local endpoints <https://kubernetes.io/docs/tasks/access-application-cluster/     |
|                 | create-external-load-balancer/#preserving-the-client-source-ip>`_ (it can influence the   |
|                 | load balancing effectiveness)                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-replicasservicetype:                                                          |
|                 |                                                                                           |
| **Key**         | `haproxy.replicasServiceType <operator.html#haproxy-replicasservicetype>`_                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``ClusterIP``                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Specifies the type of `Kubernetes Service                                                 |
|                 | <https://kubernetes.io/docs/concepts/services-networking/service/                         |
|                 | #publishing-services-service-types>`_ to be used for HAProxy replicas                     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-replicasexternaltrafficpolicy:                                                |
|                 |                                                                                           |
| **Key**         | `haproxy.replicasExternalTrafficPolicy                                                    |
|                 | <operator.html#haproxy-replicasexternaltrafficpolicy>`_                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``Cluster``                                                                               |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Specifies whether Service for HAProxy replicas should `route external traffic to          |
|                 | cluster-wide or to node-local endpoints                                                   |
|                 | <https://kubernetes.io/docs/tasks/access-application-cluster/                             |
|                 | create-external-load-balancer/#preserving-the-client-source-ip>`_ (it can influence the   |
|                 | load balancing effectiveness)                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-resources-requests-memory:                                                    |
|                 |                                                                                           |
| **Key**         | `haproxy.resources.requests.memory <operator.html#haproxy-resources-requests-memory>`_    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1G``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes memory requests                                                           |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_                                     |
|                 | for the main HAProxy container                                                            |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-resources-requests-cpu:                                                       |
|                 |                                                                                           |
| **Key**         | `haproxy.resources.requests.cpu <operator.html#haproxy-resources-requests-cpu>`_          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``600m``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes CPU requests                                                                  |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for the main HAProxy container      |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-resources-limits-memory:                                                      |
|                 |                                                                                           |
| **Key**         | `haproxy.resources.limits.memory <operator.html#haproxy-resources-limits-memory>`_        |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1G``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes memory limits                                                                 |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for the main HAProxy container      |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-resources-limits-cpu:                                                         |
|                 |                                                                                           |
| **Key**         | `haproxy.resources.limits.cpu <operator.html#haproxy-resources-limits-cpu>`_              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``700m``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes CPU limits                                                                    |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for the main HAProxy container      |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-envvarssecret:                                                                |
|                 |                                                                                           |
| **Key**         | `haproxy.envVarsSecret <operator.html#haproxy-envvarssecret>`_                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``my-env-var-secrets``                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | A secret with environment variables, see :ref:`faq-env` for details                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-priorityclassname:                                                            |
|                 |                                                                                           |
| **Key**         | `haproxy.priorityClassName <operator.html#haproxy-priorityclassname>`_                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``high-priority``                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes Pod Priority class                                                        |
|                 | <https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/               |
|                 | #priorityclass>`_ for HAProxy                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-schedulername:                                                                |
|                 |                                                                                           |
| **Key**         | `haproxy.schedulerName <operator.html#haproxy-schedulername>`_                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``mycustom-scheduler``                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes Scheduler                                                                 |
|                 | <https://kubernetes.io/docs/tasks/administer-cluster/configure-multiple-schedulers>`_     |
+-----------------+-------------------------------------------------------------------------------------------+ 
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-nodeselector:                                                                 |
|                 |                                                                                           |
| **Key**         | `haproxy.nodeSelector <operator.html#haproxy-nodeselector>`_                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | label                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``disktype: ssd``                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes nodeSelector                                                                  |
|                 | <https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector>`_       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-affinity-topologykey:                                                         |
|                 |                                                                                           |
| **Key**         | `haproxy.affinity.topologyKey <operator.html#haproxy-affinity-topologykey>`_              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``kubernetes.io/hostname``                                                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The Operator `topology key                                                                |
|                 | <https://kubernetes.io/docs/concepts/configuration/assign-pod-node/                       |
|                 | #affinity-and-anti-affinity>`_ node anti-affinity constraint                              |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-affinity-advanced:                                                            |
|                 |                                                                                           |
| **Key**         | `haproxy.affinity.advanced <operator.html#haproxy-affinity-advanced>`_                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | subdoc                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     |                                                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | If available it makes a `topologyKey                                                      |
|                 | <https://kubernetes.io/docs/concepts/configuration/assign-pod-node/                       |
|                 | #inter-pod-affinity-and-anti-affinity-beta-feature>`_ node affinity constraint to be      |
|                 | ignored                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-tolerations:                                                                  |
|                 |                                                                                           |
| **Key**         | `haproxy.tolerations <operator.html#haproxy-tolerations>`_                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | subdoc                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``node.alpha.kubernetes.io/unreachable``                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes Pod tolerations                                                               |
|                 | <https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/>`_               |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-poddisruptionbudget-maxunavailable:                                           |
|                 |                                                                                           |
| **Key**         | `haproxy.podDisruptionBudget.maxUnavailable                                               |
|                 | <operator.html#haproxy-poddisruptionbudget-maxunavailable>`_                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes podDisruptionBudget                                                       |
|                 | <https://kubernetes.io/docs/tasks/run-application/configure-pdb/                          |
|                 | #specifying-a-poddisruptionbudget>`_ specifies the number of Pods from the set            |
|                 | unavailable after the eviction                                                            |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-poddisruptionbudget-minavailable:                                             |
|                 |                                                                                           |
| **Key**         | `haproxy.podDisruptionBudget.minAvailable                                                 |
|                 | <operator.html#haproxy-poddisruptionbudget-minavailable>`_                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``0``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes podDisruptionBudget                                                       |
|                 | <https://kubernetes.io/docs/tasks/run-application/configure-pdb/                          |
|                 | #specifying-a-poddisruptionbudget>`_ Pods that must be available after an eviction        |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-graceperiod:                                                                  |
|                 |                                                                                           |
| **Key**         | `haproxy.gracePeriod <operator.html#haproxy-graceperiod>`_                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``30``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes grace period when terminating a Pod                                       |
|                 | <https://kubernetes.io/docs/concepts/workloads/pods/pod/#termination-of-pods>`_           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-loadbalancersourceranges:                                                     |
|                 |                                                                                           |
| **Key**         | `haproxy.loadBalancerSourceRanges <operator.html#haproxy-loadbalancersourceranges>`_      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``10.0.0.0/8``                                                                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The range of client IP addresses from which the load balancer should be reachable         |
|                 | (if not set, there is no limitations)                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-servicelabels:                                                                       |
|                 |                                                                                           |
| **Key**         | `haproxy.serviceLabels <operator.html#haproxy-servicelabels>`_                                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | label                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``rack: rack-23``                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes labels                                                                    |
|                 | <https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/>`_             |
|                 | for the load balancer Service                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-serviceannotations:                                                           |
|                 |                                                                                           |
| **Key**         | `haproxy.serviceAnnotations <operator.html#haproxy-serviceannotations>`_                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``service.beta.kubernetes.io/aws-load-balancer-backend-protocol: http``                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes annotations                                                               |
|                 | <https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/>`_        |
|                 | metadata for the load balancer Service                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-containersecuritycontext:                                                     |
|                 |                                                                                           |
| **Key**         | `haproxy.containerSecurityContext <operator.html#haproxy-containersecuritycontext>`_      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | subdoc                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``privileged: true``                                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | A custom `Kubernetes Security Context for a Container                                     |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/security-context/>`_ to be used |
|                 | instead of the default one                                                                |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-podsecuritycontext:                                                           |
|                 |                                                                                           |
| **Key**         | `haproxy.podSecurityContext <operator.html#haproxy-podsecuritycontext>`_                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | subdoc                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``fsGroup: 1001``                                                                         |
|                 |                                                                                           |
|                 | ``supplementalGroups: [1001, 1002, 1003]``                                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | A custom `Kubernetes Security Context for a Pod                                           |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/security-context/>`_ to be used |
|                 | instead of the default one                                                                |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-serviceaccountname:                                                           |
|                 |                                                                                           |
| **Key**         | `haproxy.serviceAccountName <operator.html#haproxy-serviceaccountname>`_                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``percona-xtradb-cluster-operator-workload``                                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes Service Account                                                           |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/>`_   |
|                 | for the HAProxy Pod                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-runtimeclassname:                                                             |
|                 |                                                                                           |
| **Key**         | `haproxy.runtimeClassName                                                                 |
|                 | <operator.html#haproxy-runtimeclassname>`_                                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``image-rc``                                                                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Name of the `Kubernetes Runtime Class                                                     |
|                 | <https://kubernetes.io/docs/concepts/containers/runtime-class/>`_                         |
|                 | for the HAProxy Pod                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-sidecars-image:                                                               |
|                 |                                                                                           |
| **Key**         | `haproxy.sidecars.image                                                                   |
|                 | <operator.html#haproxy-sidecars-image>`_                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``busybox``                                                                               |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Image for the                                                                             |
|                 | :ref:`custom sidecar container<faq-sidecar>`                                              |
|                 | for the HAProxy Pod                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-sidecars-command:                                                             |
|                 |                                                                                           |
| **Key**         | `haproxy.sidecars.command                                                                 |
|                 | <operator.html#haproxy-sidecars-command>`_                                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | array                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``["/bin/sh"]``                                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Command for the                                                                           |
|                 | :ref:`custom sidecar container<faq-sidecar>`                                              |
|                 | for the HAProxy Pod                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-sidecars-args:                                                                |
|                 |                                                                                           |
| **Key**         | `haproxy.sidecars.args                                                                    |
|                 | <operator.html#haproxy-sidecars-args>`_                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | array                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``["-c", "while true; do trap 'exit 0' SIGINT SIGTERM SIGQUIT SIGKILL; done;"]``          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Command arguments for the                                                                 |
|                 | :ref:`custom sidecar container<faq-sidecar>`                                              |
|                 | for the HAProxy Pod                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-sidecars-name:                                                                |
|                 |                                                                                           |
| **Key**         | `haproxy.sidecars.name                                                                    |
|                 | <operator.html#haproxy-sidecars-name>`_                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``my-sidecar-1``                                                                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Name of the                                                                               |
|                 | :ref:`custom sidecar container<faq-sidecar>`                                              |
|                 | for the HAProxy Pod                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-sidecars-resources-requests-memory:                                           |
|                 |                                                                                           |
| **Key**         | `haproxy.sidecars.resources.requests.memory                                               |
|                 | <operator.html#haproxy-sidecars-resources-requests-memory>`_                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1G``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes memory requests                                                           |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_                                     |
|                 | for the sidecar HAProxy containers                                                        |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-sidecars-resources-requests-cpu:                                              |
|                 |                                                                                           |
| **Key**         | `haproxy.sidecars.resources.requests.cpu                                                  |
|                 | <operator.html#haproxy-sidecars-resources-requests-cpu>`_                                 |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``500m``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes CPU requests                                                                  |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for the sidecar HAProxy containers  |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-sidecars-resources-limits-memory:                                             |
|                 |                                                                                           |
| **Key**         | `haproxy.sidecars.resources.limits.memory                                                 |
|                 | <operator.html#haproxy-sidecars-resources-limits-memory>`_                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``2G``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes memory limits                                                                 |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for the sidecar HAProxy containers  |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _haproxy-sidecars-resources-limits-cpu:                                                |
|                 |                                                                                           |
| **Key**         | `haproxy.sidecars.resources.limits.cpu                                                    |
|                 | <operator.html#haproxy-sidecars-resources-limits-cpu>`_                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``600m``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes CPU limits                                                                    |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for the sidecar HAProxy containers  |
+-----------------+-------------------------------------------------------------------------------------------+

.. _operator.proxysql-section:

`ProxySQL Section <operator.html#operator-proxysql-section>`_
--------------------------------------------------------------------------------

The ``proxysql`` section in the `deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/main/deploy/cr.yaml>`__ file contains
configuration options for the ProxySQL daemon.

.. tabularcolumns:: |p{2cm}|p{13.6cm}|

+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-enabled:                                                                     |
|                 |                                                                                           |
| **Key**         | `proxysql.enabled <operator.html#proxysql-enabled>`_                                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | boolean                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``false``                                                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Enables or disables `load balancing with ProxySQL                                         |
|                 | <https://www.percona.com/doc/percona-xtradb-cluster/5.7/howtos/proxysql.html>`_ `Services |
|                 | <https://kubernetes.io/docs/concepts/services-networking/service/>`_                      |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-size:                                                                        |
|                 |                                                                                           |
| **Key**         | `proxysql.size <operator.html#proxysql-size>`_                                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``2``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The number of the ProxySQL daemons `to provide load balancing                             |
|                 | <https://www.percona.com/doc/percona-xtradb-cluster/5.7/howtos/proxysql.html>`__. It      |
|                 | should be 2 or more unless the ``spec.allowUnsafeConfigurations`` key is set to true.     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-image:                                                                       |
|                 |                                                                                           |
| **Key**         | `proxysql.image <operator.html#proxysql-image>`_                                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``percona/percona-xtradb-cluster-operator:{{{release}}}-proxysql``                               |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | ProxySQL Docker image to use                                                              |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-imagepullpolicy:                                                             |
|                 |                                                                                           |
| **Key**         | `proxysql.imagePullPolicy <operator.html#_proxysql-imagepullpolicy>`_                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``Always``                                                                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `policy used to update images <https://kubernetes.io/docs/concepts/containers/images/ |
|                 | #updating-images>`_                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-imagepullsecrets-name:                                                       |
|                 |                                                                                           |
| **Key**         | `proxysql.imagePullSecrets.name <operator.html#proxysql-imagepullsecrets-name>`_          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``private-registry-credentials``                                                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes imagePullSecrets                                                          |
|                 | <https://kubernetes.io/docs/concepts/configuration/secret/#using-imagepullsecrets>`_ for  |
|                 | the ProxySQL image                                                                        |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-readinessdelaysec:                                                           |
|                 |                                                                                           |
| **Key**         | `proxysql.readinessDelaySec <operator.html#proxysql-readinessdelaysec>`_                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``15``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Adds a delay before a run check to verify the application is ready to process traffic     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-livenessdelaysec:                                                            |
|                 |                                                                                           |
| **Key**         | `proxysql.livenessDelaySec <operator.html#proxysql-livenessdelaysec>`_                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``300``                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Adds a delay before the run check ensures the application is healthy and capable of       |
|                 | processing requests                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-configuration:                                                               |
|                 |                                                                                           |
| **Key**         | `proxysql.configuration <operator.html#proxysql-configuration>`_                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     |                                                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The :ref:`custom ProxySQL configuration file<proxysql-conf-custom>` contents              |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-annotations:                                                                 |
|                 |                                                                                           |
| **Key**         | `proxysql.annotations <operator.html#proxysql-annotations>`_                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | label                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``iam.amazonaws.com/role: role-arn``                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes annotations                                                               |
|                 | <https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/>`_        |
|                 | metadata                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-labels:                                                                      |
|                 |                                                                                           |
| **Key**         | `proxysql.labels <operator.html#proxysql-labels>`_                                        |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | label                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``rack: rack-22``                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Labels are key-value pairs attached to objects                                           |
|                 | <https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/>`_             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-servicetype:                                                                 |
|                 |                                                                                           |
| **Key**         | `proxysql.serviceType <operator.html#proxysql-servicetype>`_                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``ClusterIP``                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Specifies the type of `Kubernetes Service                                                 |
|                 | <https://kubernetes.io/docs/concepts/services-networking/service/                         |
|                 | #publishing-services-service-types>`_ to be used                                          |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-externaltrafficpolicy:                                                       |
|                 |                                                                                           |
| **Key**         | `proxysql.externalTrafficPolicy <operator.html#proxysql-externaltrafficpolicy>`_          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``Cluster``                                                                               |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Specifies whether Service should `route external traffic to cluster-wide or node-local    |
|                 | endpoints <https://kubernetes.io/docs/tasks/access-application-cluster/                   |
|                 | create-external-load-balancer/#preserving-the-client-source-ip>`_ (it can influence the   |
|                 | load balancing effectiveness)                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-resources-requests-memory:                                                   |
|                 |                                                                                           |
| **Key**         | `proxysql.resources.requests.memory <operator.html#proxysql-resources-requests-memory>`_  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1G``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes memory requests                                                           |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_                                     |
|                 | for the main ProxySQL container                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-resources-requests-cpu:                                                      |
|                 |                                                                                           |
| **Key**         | `proxysql.resources.requests.cpu <operator.html#proxysql-resources-requests-cpu>`_        |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``600m``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes CPU requests                                                                  |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for the main ProxySQL container     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-resources-limits-memory:                                                     |
|                 |                                                                                           |
| **Key**         | `proxysql.resources.limits.memory <operator.html#proxysql-resources-limits-memory>`_      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1G``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes memory limits                                                                 |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for the main ProxySQL container     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-resources-limits-cpu:                                                        |
|                 |                                                                                           |
| **Key**         | `proxysql.resources.limits.cpu <operator.html#proxysql-resources-limits-cpu>`_            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``700m``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes CPU limits                                                                    |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for the main ProxySQL container     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-envvarssecret:                                                               |
|                 |                                                                                           |
| **Key**         | `proxysql.envVarsSecret <operator.html#proxysql-envvarssecret>`_                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``my-env-var-secrets``                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | A secret with environment variables, see :ref:`faq-env` for details                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-priorityclassname:                                                           |
|                 |                                                                                           |
| **Key**         | `proxysql.priorityClassName <operator.html#proxysql-priorityclassname>`_                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``high-priority``                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes Pod Priority class                                                        |
|                 | <https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/               |
|                 | #priorityclass>`_ for ProxySQL                                                            |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-schedulername:                                                               |
|                 |                                                                                           |
| **Key**         | `proxysql.schedulerName <operator.html#proxysql-schedulername>`_                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``mycustom-scheduler``                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes Scheduler                                                                 |
|                 | <https://kubernetes.io/docs/tasks/administer-cluster/configure-multiple-schedulers>`_     |
+-----------------+-------------------------------------------------------------------------------------------+ 
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-nodeselector:                                                                |
|                 |                                                                                           |
| **Key**         | `proxysql.nodeSelector <operator.html#proxysql-nodeselector>`_                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | label                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``disktype: ssd``                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes nodeSelector                                                                  |
|                 | <https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector>`_       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-affinity-topologykey:                                                        |
|                 |                                                                                           |
| **Key**         | `proxysql.affinity.topologyKey <operator.html#proxysql-affinity-topologykey>`_            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``kubernetes.io/hostname``                                                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The Operator `topology key                                                                |
|                 | <https://kubernetes.io/docs/concepts/configuration/assign-pod-node/                       |
|                 | #affinity-and-anti-affinity>`_ node anti-affinity constraint                              |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-affinity-advanced:                                                           |
|                 |                                                                                           |
| **Key**         | `proxysql.affinity.advanced <operator.html#proxysql-affinity-advanced>`_                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | subdoc                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     |                                                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | If available it makes a `topologyKey                                                      |
|                 | <https://kubernetes.io/docs/concepts/configuration/assign-pod-node/                       |
|                 | #inter-pod-affinity-and-anti-affinity-beta-feature>`_ node affinity constraint to be      |
|                 | ignored                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-tolerations:                                                                 |
|                 |                                                                                           |
| **Key**         | `proxysql.tolerations <operator.html#proxysql-tolerations>`_                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | subdoc                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``node.alpha.kubernetes.io/unreachable``                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes Pod tolerations                                                               |
|                 | <https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/>`_               |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-volumespec-emptydir:                                                         |
|                 |                                                                                           |
| **Key**         | `proxysql.volumeSpec.emptyDir <operator.html#proxysql-volumespec-emptydir>`_              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``{}``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes emptyDir volume                                                           |
|                 | <https://kubernetes.io/docs/concepts/storage/volumes/#emptydir>`_ The directory created   |
|                 | on a node and accessible to the Percona XtraDB Cluster Pod containers                     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-volumespec-hostpath-path:                                                    |
|                 |                                                                                           |
| **Key**         | `proxysql.volumeSpec.hostPath.path <operator.html#proxysql-volumespec-hostpath-path>`_    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``/data``                                                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes hostPath <https://kubernetes.io/docs/concepts/storage/volumes/#hostpath>`_    |
|                 | The volume that mounts a directory from the host node's filesystem into your Pod. The     |
|                 | path property is required                                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-volumespec-hostpath-type:                                                    |
|                 |                                                                                           |
| **Key**         | `proxysql.volumeSpec.hostPath.type <operator.html#proxysql-volumespec-hostpath-type>`_    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``Directory``                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes hostPath <https://kubernetes.io/docs/concepts/storage/volumes/            |
|                 | #hostpath>`_. An optional property for the hostPath                                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-volumespec-persistentvolumeclaim-storageclassname:                           |
|                 |                                                                                           |
| **Key**         | `proxysql.volumeSpec.persistentVolumeClaim.storageClassName                               |
|                 | <operator.html#proxysql-volumespec-persistentvolumeclaim-storageclassname>`_              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``standard``                                                                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Set the `Kubernetes storage class                                                         |
|                 | <https://kubernetes.io/docs/concepts/storage/storage-classes/>`_ to use with the Percona  |
|                 | XtraDB Cluster `PersistentVolumeClaim                                                     |
|                 | <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims>`_|
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-volumespec-persistentvolumeclaim-accessmodes:                                |
|                 |                                                                                           |
| **Key**         | `proxysql.volumeSpec.persistentVolumeClaim.accessModes                                    |
|                 | <operator.html#proxysql-volumespec-persistentvolumeclaim-accessmodes>`_                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | array                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``[ReadWriteOnce]``                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes PersistentVolumeClaim                                                     |
|                 | <https://kubernetes.io/docs/concepts/storage/persistent-volumes/                          |
|                 | #persistentvolumeclaims>`_ access modes for the Percona XtraDB cluster                    |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-volumespec-resources-requests-storage:                                       |
|                 |                                                                                           |
| **Key**         | `proxysql.volumeSpec.resources.requests.storage                                           |
|                 | <operator.html#proxysql-volumespec-resources-requests-storage>`_                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``6Gi``                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes PersistentVolumeClaim                                                     |
|                 | <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#                         |
|                 | persistentvolumeclaims>`_ size for the Percona XtraDB cluster                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-poddisruptionbudget-maxunavailable:                                          |
|                 |                                                                                           |
| **Key**         | `proxysql.podDisruptionBudget.maxUnavailable                                              |
|                 | <operator.html#proxysql-poddisruptionbudget-maxunavailable>`_                             |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes podDisruptionBudget                                                       |
|                 | <https://kubernetes.io/docs/tasks/run-application/configure-pdb/                          |
|                 | #specifying-a-poddisruptionbudget>`_ specifies the number of Pods from the set            |
|                 | unavailable after the eviction                                                            |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-poddisruptionbudget-minavailable:                                            |
|                 |                                                                                           |
| **Key**         | `proxysql.podDisruptionBudget.minAvailable                                                |
|                 | <operator.html#proxysql-poddisruptionbudget-minavailable>`_                               |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``0``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes podDisruptionBudget                                                       |
|                 | <https://kubernetes.io/docs/tasks/run-application/configure-pdb/                          |
|                 | #specifying-a-poddisruptionbudget>`_ Pods that must be available after an eviction        |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-graceperiod:                                                                 |
|                 |                                                                                           |
| **Key**         | `proxysql.gracePeriod <operator.html#proxysql-graceperiod>`_                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``30``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes grace period when terminating a Pod                                       |
|                 | <https://kubernetes.io/docs/concepts/workloads/pods/pod/#termination-of-pods>`_           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-loadbalancersourceranges:                                                    |
|                 |                                                                                           |
| **Key**         | `proxysql.loadBalancerSourceRanges <operator.html#proxysql-loadbalancersourceranges>`_    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``10.0.0.0/8``                                                                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The range of client IP addresses from which the load balancer should be reachable         |
|                 | (if not set, there is no limitations)                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-servicelabels:                                                               |
|                 |                                                                                           |
| **Key**         | `proxysql.serviceLabels <operator.html#proxysql-servicelabels>`_                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | label                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``rack: rack-23``                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes labels                                                                    |
|                 | <https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/>`_             |
|                 | for the load balancer Service                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-serviceannotations:                                                          |
|                 |                                                                                           |
| **Key**         | `proxysql.serviceAnnotations <operator.html#proxysql-serviceannotations>`_                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``service.beta.kubernetes.io/aws-load-balancer-backend-protocol: http``                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes annotations                                                               |
|                 | <https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/>`_        |
|                 | metadata for the load balancer Service                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-containersecuritycontext:                                                    |
|                 |                                                                                           |
| **Key**         | `proxysql.containerSecurityContext <operator.html#proxysql-containersecuritycontext>`_    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | subdoc                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``privileged: true``                                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | A custom `Kubernetes Security Context for a Container                                     |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/security-context/>`_ to be used |
|                 | instead of the default one                                                                |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-podsecuritycontext:                                                          |
|                 |                                                                                           |
| **Key**         | `proxysql.podSecurityContext <operator.html#proxysql-podsecuritycontext>`_                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | subdoc                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``fsGroup: 1001``                                                                         |
|                 |                                                                                           |
|                 | ``supplementalGroups: [1001, 1002, 1003]``                                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | A custom `Kubernetes Security Context for a Pod                                           |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/security-context/>`_ to be used |
|                 | instead of the default one                                                                |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-serviceaccountname:                                                          |
|                 |                                                                                           |
| **Key**         | `proxysql.serviceAccountName <operator.html#proxysql-serviceaccountname>`_                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``percona-xtradb-cluster-operator-workload``                                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes Service Account                                                           |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/>`_   |
|                 | for the ProxySQL Pod                                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-runtimeclassname:                                                            |
|                 |                                                                                           |
| **Key**         | `proxysql.runtimeClassName                                                                |
|                 | <operator.html#proxysql-runtimeclassname>`_                                               |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``image-rc``                                                                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Name of the `Kubernetes Runtime Class                                                     |
|                 | <https://kubernetes.io/docs/concepts/containers/runtime-class/>`_                         |
|                 | for the ProxySQL Pod                                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-sidecars-image:                                                              |
|                 |                                                                                           |
| **Key**         | `proxysql.sidecars.image                                                                  |
|                 | <operator.html#proxysql-sidecars-image>`_                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``busybox``                                                                               |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Image for the                                                                             |
|                 | :ref:`custom sidecar container<faq-sidecar>`                                              |
|                 | for the ProxySQL Pod                                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-sidecars-command:                                                            |
|                 |                                                                                           |
| **Key**         | `proxysql.sidecars.command                                                                |
|                 | <operator.html#proxysql-sidecars-command>`_                                               |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | array                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``["/bin/sh"]``                                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Command for the                                                                           |
|                 | :ref:`custom sidecar container<faq-sidecar>`                                              |
|                 | for the ProxySQL Pod                                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-sidecars-args:                                                               |
|                 |                                                                                           |
| **Key**         | `proxysql.sidecars.args                                                                   |
|                 | <operator.html#proxysql-sidecars-args>`_                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | array                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``["-c", "while true; do trap 'exit 0' SIGINT SIGTERM SIGQUIT SIGKILL; done;"]``          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Command arguments for the                                                                 |
|                 | :ref:`custom sidecar container<faq-sidecar>`                                              |
|                 | for the ProxySQL Pod                                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-sidecars-name:                                                               |
|                 |                                                                                           |
| **Key**         | `proxysql.sidecars.name                                                                   |
|                 | <operator.html#proxysql-sidecars-name>`_                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``my-sidecar-1``                                                                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Name of the                                                                               |
|                 | :ref:`custom sidecar container<faq-sidecar>`                                              |
|                 | for the ProxySQL Pod                                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-sidecars-resources-requests-memory:                                          |
|                 |                                                                                           |
| **Key**         | `proxysql.sidecars.resources.requests.memory                                              |
|                 | <operator.html#proxysql-sidecars-resources-requests-memory>`_                             |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1G``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes memory requests                                                           |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_                                     |
|                 | for the sidecar ProxySQL containers                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-sidecars-resources-requests-cpu:                                             |
|                 |                                                                                           |
| **Key**         | `proxysql.sidecars.resources.requests.cpu                                                 |
|                 | <operator.html#proxysql-sidecars-resources-requests-cpu>`_                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``500m``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes CPU requests                                                                  |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for the sidecar ProxySQL containers |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-sidecars-resources-limits-memory:                                            |
|                 |                                                                                           |
| **Key**         | `proxysql.sidecars.resources.limits.memory                                                |
|                 | <operator.html#proxysql-sidecars-resources-limits-memory>`_                               |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``2G``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes memory limits                                                                 |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for the sidecar ProxySQL containers |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _proxysql-sidecars-resources-limits-cpu:                                               |
|                 |                                                                                           |
| **Key**         | `proxysql.sidecars.resources.limits.cpu                                                   |
|                 | <operator.html#proxysql-sidecars-resources-limits-cpu>`_                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``600m``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes CPU limits                                                                    |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for the sidecar ProxySQL containers |
+-----------------+-------------------------------------------------------------------------------------------+

.. _operator.logcollector-section:

`Log Collector Section <operator.html#operator-logcollector-section>`_
--------------------------------------------------------------------------------

The ``logcollector`` section in the `deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/main/deploy/cr.yaml>`__
file contains configuration options for `Fluent Bit Log Collector <https://fluentbit.io>`_.

.. tabularcolumns:: |p{2cm}|p{13.6cm}|

+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _logcollector-enabled:                                                                 |
|                 |                                                                                           |
| **Key**         | `logcollector.enabled <operator.html#logcollector-enabled>`_                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | boolean                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``true``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Enables or disables :ref:`cluster-level logging with Fluent Bit<debug-images-logs>`       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _logcollector-image:                                                                   |
|                 |                                                                                           |
| **Key**         | `logcollector.image <operator.html#logcollector-image>`_                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``percona/percona-xtradb-cluster-operator:1.6.0-logcollector``                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Log Collector Docker image to use                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _logcollector-configuration:                                                           |
|                 |                                                                                           |
| **Key**         | `logcollector.configuration <operator.html#logcollector-configuration>`_                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       |  subdoc                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     |                                                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Additional configuration options (see `Fluent Bit official documentation                  |
|                 | <https://docs.fluentbit.io/manual/administration/configuring-fluent-bit/                  |
|                 | configuration-file>`_ for details)                                                        |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _logcollector-resources-requests-memory:                                               |
|                 |                                                                                           |
| **Key**         | `logcollector.resources.requests.memory                                                   |
|                 | <operator.html#logcollector-resources-requests-memory>`_                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``100M``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes memory requests                                                           |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_                                     |
|                 | for a Log Collector container                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _logcollector-resources-requests-cpu:                                                  |
|                 |                                                                                           |
| **Key**         | `logcollector.resources.requests.cpu <operator.html#logcollector-resources-requests-cpu>`_|
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``200m``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes CPU requests                                                                  |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for a Log collector container       |
+-----------------+-------------------------------------------------------------------------------------------+

.. _operator.pmm-section:

`PMM Section <operator.html#operator-pmm-section>`_
--------------------------------------------------------------------------------

The ``pmm`` section in the `deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/main/deploy/cr.yaml>`__ file contains configuration
options for Percona Monitoring and Management.

.. tabularcolumns:: |p{2cm}|p{13.6cm}|

+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pmm-enabled:                                                                          |
|                 |                                                                                           |
| **Key**         | `pmm.enabled <operator.html#pmm-enabled>`_                                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | boolean                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``false``                                                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Enables or disables `monitoring Percona XtraDB cluster with PMM                           |
|                 | <https://www.percona.com/doc/percona-xtradb-cluster/5.7/manual/monitoring.html>`_         |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pmm-image:                                                                            |
|                 |                                                                                           |
| **Key**         | `pmm.image <operator.html#pmm-image>`_                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``percona/pmm-client:{{{pmm2recommended}}}``                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | PMM client Docker image to use                                                            |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pmm-serverhost:                                                                       |
|                 |                                                                                           |
| **Key**         | `pmm.serverHost <operator.html#pmm-serverhost>`_                                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       |  string                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     |  ``monitoring-service``                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Address of the PMM Server to collect data from the cluster                                |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pmm-serveruser:                                                                       |
|                 |                                                                                           |
| **Key**         | `pmm.serverUser <operator.html#pmm-serveruser>`_                                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``admin``                                                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `PMM Serve_User                                                                       |
|                 | <https://www.percona.com/doc/percona-monitoring-and-management/glossary.option.html>`_.   |
|                 | The PMM Server password should be configured using Secrets                                |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pmm-resources-requests-memory:                                                        |
|                 |                                                                                           |
| **Key**         | `pmm.resources.requests.memory <operator.html#pmm-resources-requests-memory>`_            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``150M``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes memory requests                                                           |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_                                     |
|                 | for a PMM container                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pmm-resources-requests-cpu:                                                           |
|                 |                                                                                           |
| **Key**         | `pmm.resources.requests.cpu <operator.html#pmm-resources-requests-cpu>`_                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``300m``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes CPU requests                                                                  |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for a PMM container                 |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pmm-pxcparams:                                                                        |
|                 |                                                                                           |
| **Key**         | `pmm.pxcParams <operator.html#pmm-pxcparams>`_                                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``--disable-tablestats-limit=2000``                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Additional parameters which will be passed to the `pmm-admin add mysql                    |
|                 | <https://www.percona.com/doc/percona-monitoring-and-management/2.x/setting-up/client/     |
|                 | mysql.html#adding-mysql-service-monitoring>`_ command for ``pxc`` Pods                    |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _pmm-proxysqlparams:                                                                   |
|                 |                                                                                           |
| **Key**         | `pmm.proxysqlParams <operator.html#pmm-proxysqlparams>`_                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value Type**  | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``--custom-labels=CUSTOM-LABELS``                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Additional parameters which will be passed to the `pmm-admin add mysql                    |
|                 | <https://www.percona.com/doc/percona-monitoring-and-management/2.x/setting-up/client/     |
|                 | mysql.html#adding-mysql-service-monitoring>`_ command for ``proxysql`` Pods               |
+-----------------+-------------------------------------------------------------------------------------------+

.. _operator.backup-section:

`Backup Section <operator.html#operator-backup-section>`_
--------------------------------------------------------------------------------

The ``backup`` section in the
`deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/main/deploy/cr.yaml>`__
file contains the following configuration options for the regular
Percona XtraDB Cluster backups.

.. tabularcolumns:: |p{2cm}|p{13.6cm}|

+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-image:                                                                         |
|                 |                                                                                           |
| **Key**         | `backup.image <operator.html#backup-image>`_                                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``percona/percona-xtradb-cluster-operator:{{{release}}}-backup``                                 |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The Percona XtraDB cluster Docker image to use for the backup                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-imagepullsecrets-name:                                                         |
|                 |                                                                                           |
| **Key**         | `backup.imagePullSecrets.name <operator.html#backup-imagepullsecrets-name>`_              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``private-registry-credentials``                                                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes imagePullSecrets                                                          |
|                 | <https://kubernetes.io/docs/concepts/configuration/secret/#using-imagepullsecrets>`_ for  |
|                 | the specified image                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-type:                                                                 |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.type <operator.html#backup-storages-type>`_               |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``s3``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The cloud storage type used for backups. Only ``s3`` and ``filesystem`` types are         |
|                 | supported                                                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-verifytls:                                                            |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.verifyTLS <operator.html#backup-storages-verifytls>`_     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | boolean                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``true``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Enable or disable verification of the storage server TLS certificate. Disabling it may be |
|                 | useful e.g. to skip TLS verification for private S3-compatible storage with a self-issued |
|                 | certificate.                                                                              |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-s3-credentialssecret:                                                 |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.s3.credentialsSecret                                      |
|                 | <operator.html#backup-storages-s3-credentialssecret>`_                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``my-cluster-name-backup-s3``                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes secret <https://kubernetes.io/docs/concepts/configuration/secret/>`_ for  |
|                 | backups. It should contain ``AWS_ACCESS_KEY_ID`` and ``AWS_SECRET_ACCESS_KEY`` keys.      |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-s3-bucket:                                                            |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.s3.bucket <operator.html#backup-storages-s3-bucket>`_     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     |                                                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Amazon S3 bucket <https://docs.aws.amazon.com/AmazonS3/latest/dev/UsingBucket.html>`_|
|                 | name for backups                                                                          |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-s3-region:                                                            |
|                 |                                                                                           |
| **Key**         | `backup.storages.s3.<storage-name>.region <operator.html#backup-storages-s3-region>`_     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``us-east-1``                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `AWS region <https://docs.aws.amazon.com/general/latest/gr/rande.html>`_ to use.      |
|                 | Please note **this option is mandatory** for Amazon and all S3-compatible storages        |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-s3-endpointurl:                                                       |
|                 |                                                                                           |
| **Key**         | `backup.storages.s3.<storage-name>.endpointUrl                                            |
|                 | <operator.html#backup-storages-s3-endpointurl>`_                                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     |                                                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The endpoint URL of the S3-compatible storage to be used (not needed for the original     |
|                 | Amazon S3 cloud)                                                                          |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-persistentvolumeclaim-type:                                           |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.persistentVolumeClaim.type                                |
|                 | <operator.html#backup-storages-persistentvolumeclaim-type>`_                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``filesystem``                                                                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The persistent volume claim storage type                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-volume-persistentvolumeclaim-storageclassname:                        |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.persistentVolumeClaim.storageClassName                    |
|                 | <operator.html#backup-storages-volume-persistentvolumeclaim-storageclassname>`_           |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``standard``                                                                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Set the `Kubernetes Storage Class                                                         |
|                 | <https://kubernetes.io/docs/concepts/storage/storage-classes/>`_ to use with the Percona  |
|                 | XtraDB Cluster backups `PersistentVolumeClaims                                            |
|                 | <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims>`_|
|                 | for the ``filesystem`` storage type                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-volume-persistentvolumeclaim-accessmodes:                             |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.volume.persistentVolumeClaim.accessModes                  |
|                 | <operator.html#backup-storages-volume-persistentvolumeclaim-accessmodes>`_                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | array                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``[ReadWriteOne]``                                                                        |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes PersistentVolume access modes                                             |
|                 | <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#access-modes>`_          |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-volume-persistentvolumeclaim-resources-requests-storage:              |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.volume.persistentVolumeClaim.resources.requests.storage   |
|                 | <operator.html#backup-storages-volume-persistentvolumeclaim-resources-requests-storage>`_ |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``6Gi``                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Storage size for the PersistentVolume                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-annotations:                                                          |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.annotations <operator.html#backup-storages-annotations>`_ |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | label                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``iam.amazonaws.com/role: role-arn``                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes annotations                                                               |
|                 | <https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/>`_        |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-labels:                                                               |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.labels <operator.html#backup-storages-labels>`_           |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | label                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``rack: rack-22``                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Labels are key-value pairs attached to objects                                           |
|                 | <https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/>`_             |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-resources-requests-memory:                                            |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.resources.requests.memory                                 |
|                 | <operator.html#backup-storages-resources-requests-memory>`_                               |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1G``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes memory requests                                                           |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_                                     |
|                 | for a Percona XtraDB Cluster container                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-resources-requests-cpu:                                               |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.resources.requests.cpu                                    |
|                 | <operator.html#backup-storages-resources-requests-cpu>`_                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``600m``                                                                                  |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes CPU requests                                                                  |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for a Percona XtraDB Cluster        |
|                 | container                                                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-resources-limits-memory:                                              |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.resources.limits.memory                                   |
|                 | <operator.html#backup-storages-resources-limits-memory>`_                                 |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``1G``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes memory limits                                                                 |
|                 | <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/    |
|                 | #resource-requests-and-limits-of-pod-and-container>`_ for a Percona XtraDB Cluster        |
|                 | container                                                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-nodeselector:                                                         |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.nodeSelector                                              |
|                 | <operator.html#backup-storages-nodeselector>`_                                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | label                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``disktype: ssd``                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes nodeSelector                                                                  |
|                 | <https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector>`_       |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-affinity-nodeaffinity:                                                |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.affinity.nodeAffinity                                     |
|                 | <operator.html#backup-storages-affinity-nodeaffinity>`_                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | subdoc                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     |                                                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The Operator `node affinity                                                               |
|                 | <https://kubernetes.io/docs/concepts/configuration/assign-pod-node/                       |
|                 | #affinity-and-anti-affinity>`_ constraint                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-tolerations:                                                          |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.tolerations <operator.html#backup-storages-tolerations>`_ |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | subdoc                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``backupWorker``                                                                          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | `Kubernetes Pod tolerations                                                               |
|                 | <https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/>`_               |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-priorityclassname:                                                    |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.priorityClassName                                         |
|                 | <operator.html#backup-storages-priorityclassname>`_                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``high-priority``                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes Pod priority class                                                        |
|                 | <https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/               |
|                 | #priorityclass>`_                                                                         |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-schedulername:                                                        |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.schedulerName                                             |
|                 | <operator.html#backup-storages-schedulername>`_                                           |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``mycustom-scheduler``                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The `Kubernetes Scheduler                                                                 |
|                 | <https://kubernetes.io/docs/tasks/administer-cluster/configure-multiple-schedulers>`_     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-containersecuritycontext:                                             |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.containerSecurityContext                                  |
|                 | <operator.html#backup-storages-containersecuritycontext>`_                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | subdoc                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``privileged: true``                                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | A custom `Kubernetes Security Context for a Container                                     |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/security-context/>`_ to be used |
|                 | instead of the default one                                                                |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-storages-podsecuritycontext:                                                   |
|                 |                                                                                           |
| **Key**         | `backup.storages.<storage-name>.podSecurityContext                                        |
|                 | <operator.html#backup-storages-podsecuritycontext>`_                                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | subdoc                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``fsGroup: 1001``                                                                         |
|                 |                                                                                           |
|                 | ``supplementalGroups: [1001, 1002, 1003]``                                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | A custom `Kubernetes Security Context for a Pod                                           |
|                 | <https://kubernetes.io/docs/tasks/configure-pod-container/security-context/>`_ to be used |
|                 | instead of the default one                                                                |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-schedule-name:                                                                 |
|                 |                                                                                           |
| **Key**         | `backup.schedule.name <operator.html#backup-schedule-name>`_                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``sat-night-backup``                                                                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The backup name                                                                           |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-schedule-schedule:                                                             |
|                 |                                                                                           |
| **Key**         | `backup.schedule.schedule <operator.html#backup-schedule-schedule>`_                      |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``0 0 * * 6``                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Scheduled time to make a backup specified in the                                          |
|                 | `crontab format <https://en.wikipedia.org/wiki/Cron>`_                                    |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-schedule-keep:                                                                 |
|                 |                                                                                           |
| **Key**         | `backup.schedule.keep <operator.html#backup-schedule-keep>`_                              |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``3``                                                                                     |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The amount of most recent backups to store. Older backups are automatically deleted.      |
|                 | Set ``keep`` to zero or completely remove it to disable automatic deletion of backups     |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-schedule-storagename:                                                          |
|                 |                                                                                           |
| **Key**         | `backup.schedule.storageName <operator.html#backup-schedule-storagename>`_                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``s3-us-west``                                                                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The name of the storage for the backups configured in the ``storages`` or ``fs-pvc``      |
|                 | subsection                                                                                |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-pitr-enabled:                                                                  |
|                 |                                                                                           |
| **Key**         | `backup.pitr.enabled <operator.html#backup-pitr-enabled>`_                                |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | boolean                                                                                   |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``false``                                                                                 |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Enables or disables :ref:`point-in-time-recovery functionality<backups-pitr-binlog>`      |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-pitr-storagename:                                                              |
|                 |                                                                                           |
| **Key**         | `backup.pitr.storageName <operator.html#backup-pitr-storagename>`_                        |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | string                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``s3-us-west``                                                                            |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | The name of the storage for the backups configured in the ``storages``                    |
|                 | subsection, which will be reused to store binlog for point-in-time-recovery               |
+-----------------+-------------------------------------------------------------------------------------------+
|                                                                                                             |
+-----------------+-------------------------------------------------------------------------------------------+
|                 | .. _backup-pitr-timebetweenuploads:                                                       |
|                 |                                                                                           |
| **Key**         | `backup.pitr.timeBetweenUploads <operator.html#backup-pitr-timebetweenuploads>`_          |
+-----------------+-------------------------------------------------------------------------------------------+
| **Value**       | int                                                                                       |
+-----------------+-------------------------------------------------------------------------------------------+
| **Example**     | ``60``                                                                                    |
+-----------------+-------------------------------------------------------------------------------------------+
| **Description** | Seconds between running the binlog uploader                                               |
+-----------------+-------------------------------------------------------------------------------------------+
