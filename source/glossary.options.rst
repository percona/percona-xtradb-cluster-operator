The Operator options glossary
=============================

.. glossary::

   accessModes

     ..

      :ref:`backup.storages.STORAGE_NAME.volume.persistentVolumeClaim.accessModes<backup-storages-volume-persistentvolumeclaim-accessmodes>`

      :ref:`proxysql.volumeSpec.persistentVolumeClaim.accessModes<proxysql-volumespec-persistentvolumeclaim-accessmodes>`

      :ref:`pxc.volumeSpec.persistentVolumeClaim.accessModes<pxc-volumespec-persistentvolumeclaim-accessmodes>`

   advanced

     ..

      :ref:`haproxy.affinity.advanced<haproxy-affinity-advanced>`

      :ref:`proxysql.affinity.advanced<proxysql-affinity-advanced>`

      :ref:`pxc.affinity.advanced<pxc-affinity-advanced>`

   annotations

     ..

      :ref:`backup.storages.STORAGE_NAME.annotations<backup-storages-annotations>`

      :ref:`haproxy.annotations<haproxy-annotations>`

      :ref:`proxysql.annotations<proxysql-annotations>`

      :ref:`pxc.schedulerName<pxc-schedulername>`
      
      :ref:`pxc.expose.annotations<pxc-expose-annotations>`

   apply

     ..

      :ref:`upgradeOptions.apply<upgradeoptions-apply>`

   args

     ..

      :ref:`haproxy.sidecars.args<haproxy-sidecars-args>`

      :ref:`proxysql.sidecars.args<proxysql-sidecars-args>`

      :ref:`pxc.sidecars.args<pxc-sidecars-args>`

   autoRecovery

     ..

      :ref:`pxc.autoRecovery<pxc-autorecovery>`

   bucket

     ..

      :ref:`backup.storages.STORAGE_NAME.s3.bucket<backup-storages-s3-bucket>`

   command

     ..

      :ref:`haproxy.sidecars.command<haproxy-sidecars-command>`

      :ref:`proxysql.sidecars.command<proxysql-sidecars-command>`

      :ref:`pxc.sidecars.command<pxc-sidecars-command>`

   configuration

     ..

      :ref:`haproxy.configuration<haproxy-configuration>`

      :ref:`logcollector.configuration<logcollector-configuration>`

      :ref:`proxysql.configuration<proxysql-configuration>`

      :ref:`pxc.configuration<pxc-configuration>`

   containerSecurityContext

     ..

      :ref:`backup.storages.STORAGE_NAME.containerSecurityContext<backup-storages-containersecuritycontext>`

      :ref:`pxc.containerSecurityContext<pxc-containersecuritycontext>`

   cpu

     ..

      :ref:`backup.storages.STORAGE_NAME.resources.requests.cpu<backup-storages-resources-requests-cpu>`

      :ref:`haproxy.resources.limits.cpu<haproxy-resources-limits-cpu>`

      :ref:`haproxy.resources.requests.cpu<haproxy-resources-requests-cpu>`

      :ref:`haproxy.sidecars.resources.limits.cpu<haproxy-sidecars-resources-limits-cpu>`

      :ref:`haproxy.sidecars.resources.requests.cpu<haproxy-sidecars-resources-requests-cpu>`

      :ref:`logcollector.resources.requests.cpu<logcollector-resources-requests-cpu>`

      :ref:`pmm.resources.requests.cpu<pmm-resources-requests-cpu>`

      :ref:`proxysql.resources.limits.cpu<proxysql-resources-limits-cpu>`

      :ref:`proxysql.resources.requests.cpu<proxysql-resources-requests-cpu>`

      :ref:`proxysql.sidecars.resources.limits.cpu<proxysql-sidecars-resources-limits-cpu>`

      :ref:`proxysql.sidecars.resources.requests.cpu<proxysql-sidecars-resources-requests-cpu>`

      :ref:`pxc.resources.limits.cpu<pxc-resources-limits-cpu>`

      :ref:`pxc.resources.requests.cpu<pxc-resources-requests-cpu>`

      :ref:`pxc.sidecars.resources.limits.cpu<pxc-sidecars-resources-limits-cpu>`

      :ref:`pxc.sidecars.resources.requests.cpu<pxc-sidecars-resources-requests-cpu>`

   credentialsSecret

     ..

      :ref:`backup.storages.STORAGE_NAME.s3.credentialsSecret<backup-storages-s3-credentialssecret>`

   gracePeriod

     ..

      :ref:`haproxy.gracePeriod<haproxy-graceperiod>`

      :ref:`proxysql.gracePeriod<proxysql-graceperiod>`

      :ref:`pxc.gracePeriod<pxc-graceperiod>`

   emptyDir

     ..

      :ref:`proxysql.volumeSpec.emptyDir<proxysql-volumespec-emptydir>`

      :ref:`pxc.volumeSpec.emptyDir<pxc-volumespec-emptydir>`

   enabled

     ..

      :ref:`backup.pitr.enabled<backup-pitr-enabled>`

      :ref:`haproxy.enabled<haproxy-enabled>`

      :ref:`logcollector.enabled<logcollector-enabled>`

      :ref:`pmm.enabled<pmm-enabled>`

      :ref:`proxysql.enabled<proxysql-enabled>`

      :ref:`pxc.expose.enabled<pxc-expose-enabled>`

   endpointUrl

     ..

      :ref:`backup.storages.s3.STORAGE_NAME.endpointUrl<backup-storages-s3-endpointurl>`

   envVarsSecret

     ..

      :ref:`haproxy.envVarsSecret<haproxy-envvarssecret>`

      :ref:`proxysql.envVarsSecret<proxysql-envvarssecret>`

      :ref:`pxc.envVarsSecret<pxc-envvarssecret>`

   ephemeral-storage

     ..

      :ref:`pxc.resources.limits.ephemeral-storage<pxc-resources-limits-ephemeral-storage>`

      :ref:`pxc.resources.requests.ephemeral-storage<pxc-resources-requests-ephemeral-storage>`

   externalTrafficPolicy

     ..

      :ref:`haproxy.externalTrafficPolicy<haproxy-externaltrafficpolicy>`

      :ref:`proxysql.externalTrafficPolicy<proxysql-externaltrafficpolicy>`

   failureThreshold

     ..

      :ref:`haproxy.readinessProbes.failureThreshold<haproxy-readinessprobes-failurethreshold>`

      :ref:`pxc.livenessProbes.failureThreshold<pxc-livenessprobes-failurethreshold>`

      :ref:`pxc.readinessProbes.failureThreshold<pxc-readinessprobes-failurethreshold>`

   group

     ..

      :ref:`issuerConf.group<operator.html#issuerconf-group>`

   host

     ..

      :ref:`pxc.replicationChannels.sourcesList.host<pxc-replicationchannels-sourceslist-host>`

   image

     ..

      :ref:`backup.image<backup-image>`

      :ref:`haproxy.image<haproxy-image>`

      :ref:`haproxy.sidecars.image<haproxy-sidecars-image>`

      :ref:`logcollector.image<logcollector-image>`

      :ref:`pmm.image<pmm-image>`

      :ref:`proxysql.image<proxysql-image>`

      :ref:`proxysql.sidecars.image<proxysql-sidecars-image>`

      :ref:`pxc.image<pxc-image>`

      :ref:`pxc.sidecars.image<pxc-sidecars-image>`

   imagePullPolicy

     ..

      :ref:`haproxy.imagePullPolicy<haproxy-imagepullpolicy>`

      :ref:`proxysql.imagePullPolicy<proxysql-imagepullpolicy>`

      :ref:`pxc.imagePullPolicy<pxc-imagepullpolicy>`

   initialDelaySeconds

     ..

      :ref:`haproxy.readinessProbes.initialDelaySeconds<haproxy-readinessprobes-initialdelayseconds>`

      :ref:`pxc.livenessProbes.initialDelaySeconds<pxc-livenessprobes-initialdelayseconds>`

      :ref:`pxc.readinessProbes.initialDelaySeconds<pxc-readinessprobes-initialdelayseconds>`

   isSource

     ..

      :ref:`pxc.replicationChannels.isSource<pxc-replicationchannels-issource>`

   keep

     ..

      :ref:`backup.schedule.keep<backup-schedule-keep>`

   kind

     ..

      :ref:`issuerConf.kind<operator.html#issuerconf-kind>`

   labels

     ..

      :ref:`backup.storages.STORAGE_NAME.labels<backup-storages-labels>`

      :ref:`haproxy.labels<haproxy-labels>`

      :ref:`proxysql.labels<proxysql-labels>`

      :ref:`pxc.labels<pxc-labels>`

   livenessDelaySec

     ..

      :ref:`pxc.livenessDelaySec<pxc-livenessdelaysec>`

   loadBalancerSourceRanges

     ..

      :ref:`haproxy.loadBalancerSourceRanges<haproxy-loadbalancersourceranges>`

      :ref:`proxysql.loadBalancerSourceRanges<proxysql-loadbalancersourceranges>`

      :ref:`pxc.expose.loadBalancerSourceRanges<pxc-expose-loadbalancersourceranges>`

   maxUnavailable

     ..

      :ref:`haproxy.podDisruptionBudget.maxUnavailable<haproxy-poddisruptionbudget-maxunavailable>`

      :ref:`proxysql.podDisruptionBudget.maxUnavailable<proxysql-poddisruptionbudget-maxunavailable>`

      :ref:`pxc.podDisruptionBudget.maxUnavailable<pxc-poddisruptionbudget-maxunavailable>`

   memory

     ..

      :ref:`backup.storages.STORAGE_NAME.resources.limits.memory<backup-storages-resources-limits-memory>`

      :ref:`backup.storages.STORAGE_NAME.resources.requests.memory<backup-storages-resources-requests-memory>`

      :ref:`haproxy.resources.limits.memory<haproxy-resources-limits-memory>`

      :ref:`haproxy.resources.requests.memory<haproxy-resources-requests-memory>`

      :ref:`haproxy.sidecars.resources.limits.memory<haproxy-sidecars-resources-limits-memory>`

      :ref:`haproxy.sidecars.resources.requests.memory<haproxy-sidecars-resources-requests-memory>`

      :ref:`logcollector.resources.requests.memory<logcollector-resources-requests-memory>`

      :ref:`pmm.resources.requests.memory<pmm-resources-requests-memory>`

      :ref:`proxysql.resources.limits.memory<proxysql-resources-limits-memory>`

      :ref:`proxysql.resources.requests.memory<proxysql-resources-requests-memory>`

      :ref:`proxysql.sidecars.resources.limits.memory<proxysql-sidecars-resources-limits-memory>`

      :ref:`proxysql.sidecars.resources.requests.memory<proxysql-sidecars-resources-requests-memory>`

      :ref:`pxc.resources.limits.memory<pxc-resources-limits-memory>`

      :ref:`pxc.resources.requests.memory<pxc-resources-requests-memory>`

      :ref:`pxc.sidecars.resources.limits.memory<pxc-sidecars-resources-limits-memory>`

      :ref:`pxc.sidecars.resources.requests.memory<pxc-sidecars-resources-requests-memory>`

   minAvailable

     ..

      :ref:`haproxy.podDisruptionBudget.minAvailable<haproxy-poddisruptionbudget-minavailable>`

      :ref:`proxysql.podDisruptionBudget.minAvailable<proxysql-poddisruptionbudget-minavailable>`

      :ref:`pxc.podDisruptionBudget.minAvailable<pxc-poddisruptionbudget-minavailable>`

   name

     ..

      :ref:`backup.imagePullSecrets.name<backup-imagepullsecrets-name>`

      :ref:`backup.schedule.name<backup-schedule-name>`

      :ref:`haproxy.imagePullSecrets.name<haproxy-imagepullsecrets-name>`

      :ref:`haproxy.sidecars.name<haproxy-sidecars-name>`

      :ref:`issuerConf.name<operator.html#issuerconf-name>`

      :ref:`proxysql.imagePullSecrets.name<proxysql-imagepullsecrets-name>`

      :ref:`proxysql.sidecars.name<proxysql-sidecars-name>`

      :ref:`pxc.imagePullSecrets.name<pxc-imagepullsecrets-name>`

      :ref:`pxc.replicationChannels.name<pxc-replicationchannels-name>`
      
      :ref:`pxc.sidecars.name<pxc-sidecars-name>`

   nodeAffinity

     ..

      :ref:`backup.storages.STORAGE_NAME.affinity.nodeAffinity<backup-storages-affinity-nodeaffinity>`

   nodeSelector

     ..

      :ref:`backup.storages.STORAGE_NAME.nodeSelector<backup-storages-nodeselector>`

      :ref:`haproxy.nodeSelector<haproxy-nodeselector>`

      :ref:`proxysql.nodeSelector<proxysql-nodeselector>`

      :ref:`pxc.nodeSelector<pxc-nodeselector>`

   path

     ..

      :ref:`proxysql.volumeSpec.hostPath.path<proxysql-volumespec-hostpath-path>`

   periodSeconds

     ..

      :ref:`haproxy.readinessProbes.periodSeconds<haproxy-readinessprobes-periodseconds>`

      :ref:`pxc.livenessProbes.periodSeconds<pxc-livenessprobes-periodseconds>`

      :ref:`pxc.readinessProbes.periodSeconds<pxc-readinessprobes-periodseconds>`

   podSecurityContext

     ..

      :ref:`backup.storages.STORAGE_NAME.podSecurityContext<backup-storages-podsecuritycontext>`

      :ref:`pxc.podSecurityContext<pxc-podsecuritycontext>`

   port

     ..

      :ref:`pxc.replicationChannels.sourcesList.port<pxc-replicationchannels-sourceslist-port>`

   priorityClassName

     ..

      :ref:`backup.storages.STORAGE_NAME.priorityClassName<backup-storages-priorityclassname>`

      :ref:`haproxy.priorityClassName<haproxy-priorityclassname>`

      :ref:`proxysql.priorityClassName<proxysql-priorityclassname>`

      :ref:`pxc.priorityClassName<pxc-priorityclassname>`

   proxysqlParams

     ..

      :ref:`pmm.proxysqlParams<pmm-proxysqlparams>`

   pxcParams

     ..

      :ref:`pmm.pxcParams<pmm-pxcparams>`

   readinessDelaySec

     ..

      :ref:`pxc.readinessDelaySec<pxc-readinessdelaysec>`

   region

     ..

      :ref:`backup.storages.s3.STORAGE_NAME.region<backup-storages-s3-region>`

   replicasExternalTrafficPolicy

     ..

      :ref:`haproxy.replicasExternalTrafficPolicy<haproxy-replicasexternaltrafficpolicy>`

   replicasServiceType

     ..

      :ref:`haproxy.replicasServiceType<haproxy-replicasservicetype>`

   runtimeClassName

     ..

      :ref:`haproxy.runtimeClassName<haproxy-runtimeclassname>`

      :ref:`proxysql.runtimeClassName<proxysql-runtimeclassname>`

      :ref:`pxc.runtimeClassName<pxc-runtimeclassname>`

   schedule

     ..

      :ref:`backup.schedule.schedule<backup-schedule-schedule>`

      :ref:`upgradeOptions.schedule<upgradeoptions-schedule>`

   schedulerName

     ..

      :ref:`backup.storages.STORAGE_NAME.schedulerName<backup-storages-schedulername>`

      :ref:`haproxy.schedulerName<haproxy-schedulername>`

      :ref:`proxysql.schedulerName<proxysql-schedulername>`

      :ref:`pxc.schedulerName<pxc-schedulername>`

   serverHost

     ..

      :ref:`pmm.serverHost<pmm-serverhost>`

   serverUser

     ..

      :ref:`pmm.serverUser<pmm-serveruser>`

   serviceAccountName

     ..

      :ref:`haproxy.serviceAccountName<haproxy-serviceaccountname>`

      :ref:`proxysql.serviceAccountName<proxysql-serviceaccountname>`

      :ref:`pxc.serviceAccountName<pxc-serviceaccountname>`

   serviceAnnotations

     ..

      :ref:`haproxy.serviceAnnotations<haproxy-serviceannotations>`

      :ref:`proxysql.serviceAnnotations<proxysql-serviceannotations>`
      
   serviceLabels

     ..

      :ref:`haproxy.serviceLabels<haproxy-servicelabels>`

      :ref:`proxysql.serviceLabels<proxysql-servicelabels>`

   serviceType

     ..

      :ref:`haproxy.serviceType<haproxy-servicetype>`

      :ref:`proxysql.serviceType<proxysql-servicetype>`

   size

     ..

      :ref:`haproxy.size<haproxy-size>`

      :ref:`proxysql.size<proxysql-size>`

      :ref:`pxc.size<pxc-size>`

      :ref:`ProxySQL<proxysql-size>`

   sourceConnectRetry
   
     ..

      :ref:`pxc.replicationChannels.configuration.sourceConnectRetry<pxc-replicationchannels-configuration-sourceconnectretry>`

   sourceRetryCount

     ..

      :ref:`pxc.replicationChannels.configuration.sourceRetryCount<pxc-replicationchannels-configuration-sourceretrycount>`


   storage

     ..

      :ref:`backup.storages.STORAGE_NAME.volume.persistentVolumeClaim.resources.requests.storage<backup-storages-volume-persistentvolumeclaim-resources-requests-storage>`

      :ref:`proxysql.volumeSpec.resources.requests.storage<proxysql-volumespec-resources-requests-storage>`

      :ref:`pxc.volumeSpec.resources.requests.storage<pxc-volumespec-resources-requests-storage>`

   storageClassName

     ..

      :ref:`backup.storages.STORAGE_NAME.persistentVolumeClaim.storageClassName<backup-storages-volume-persistentvolumeclaim-storageclassname>`

      :ref:`proxysql.volumeSpec.persistentVolumeClaim.storageClassName<proxysql-volumespec-persistentvolumeclaim-storageclassname>`

      :ref:`pxc.volumeSpec.persistentVolumeClaim.storageClassName<pxc-volumespec-persistentvolumeclaim-storageclassname>`

   storageName

     ..

      :ref:`backup.pitr.storageName<backup-pitr-storagename>`

      :ref:`backup.schedule.storageName<backup-schedule-storagename>`

   successThreshold

     ..

      :ref:`pxc.livenessProbes.successThreshold<pxc-livenessprobes-successthreshold>`

      :ref:`pxc.readinessProbes.successThreshold<pxc-readinessprobes-successthreshold>`

   timeBetweenUploads

     ..

      :ref:`backup.pitr.timeBetweenUploads<backup-pitr-timebetweenuploads>`

   timeoutSeconds

     ..

      :ref:`haproxy.readinessProbes.timeoutSeconds<haproxy-readinessprobes-timeoutseconds>`

      :ref:`pxc.livenessProbes.timeoutSeconds<pxc-livenessprobes-timeoutseconds>`

      :ref:`pxc.readinessProbes.timeoutSeconds<pxc-readinessprobes-timeoutseconds>`

   tolerations

     ..

      :ref:`backup.storages.STORAGE_NAME.tolerations<backup-storages-tolerations>`

      :ref:`haproxy.tolerations<haproxy-tolerations>`

      :ref:`proxysql.tolerations<proxysql-tolerations>`

      :ref:`pxc.tolerations<pxc-tolerations>`

   topologyKey

     ..

      :ref:`haproxy.affinity.topologyKey<haproxy-affinity-topologykey>`

      :ref:`proxysql.affinity.topologyKey<proxysql-affinity-topologykey>`

      :ref:`pxc.affinity.topologyKey<pxc-affinity-topologykey>`

   type

     ..

      :ref:`backup.storages.STORAGE_NAME.persistentVolumeClaim.type<backup-storages-persistentvolumeclaim-type>`

      :ref:`backup.storages.STORAGE_NAME.type<backup-storages-type>`

      :ref:`proxysql.volumeSpec.hostPath.type<proxysql-volumespec-hostpath-type>`

      :ref:`pxc.expose.type<pxc-expose-type>`

      :ref:`pxc.volumeSpec.hostPath.type<pxc-volumespec-hostpath-type>`

   verifyTLS

     ..

      :ref:`backup.storages.STORAGE_NAME.verifyTLS<operator.html#backup-storages-verifytls>`

   versionServiceEndpoint

     ..

      :ref:`upgradeOptions.versionServiceEndpoint<upgradeoptions-versionserviceendpoint>`

   weight

     ..

      :ref:`pxc.replicationChannels.sourcesList.weight<pxc-replicationchannels-sourceslist-weight>`

