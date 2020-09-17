.. _K8SPXC-1.6.0:qaq

================================================================================
*Percona Kubernetes Operator for Percona XtraDB Cluster* 1.6.0
================================================================================

:Date: September 30, 2020
:Installation: `Installing Percona Kubernetes Operator for Percona XtraDB Cluster <https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html#quickstart-guides>`_

New Features
================================================================================

* :jirabug:`K8SPXC-144`: Allow adding configuration options (support ConfigMaps) for ProxySQL Pods
* :jirabug:`K8SPXC-429`: Custom resource options included in backups
* :jirabug:`K8SPXC-428`: The Vault token issuer
* :jirabug:`K8SPXC-368`: Auto update system users by changing the appropriate
  secret name
* :jirabug:`K8SPXC-144`: Support ConfigMaps for ProxySQL configuration
* :jirabug:`K8SPXC-343`: Helm chart officially provided with the Operator

Improvements
================================================================================

* :jirabug:`K8SPXC-416`: Support of the HA Proxy proxy-protocol
* :jirabug:`K8SPXC-317`: The possiblity to configure the Operator's imagePullPolicy option (Thanks to user imranrazakhan for reporting this issue)

Bugs Fixed
================================================================================

* :jirabug:`K8SPXC-398`: PXC deployments submitted via the Operator API broken in v1.5.0 - network frontends cannot authenticate (Thanks to user mike.saah for reporting this issue)
* :jirabug:`K8SPXC-431`: HAProxy unable to start on OpenShift with the default cr.yaml file
* :jirabug:`K8SPXC-408`: **(improvement, not bug!)** Increase MAX_USER_CONNECTIONS for ProxySQL monitor user from 10 to 100
* :jirabug:`K8SPXC-391`: HA Proxy and PMM cannot be enabled at the same time (Thanks to user rf_enigm for reporting this issue)
* :jirabug:`K8SPXC-390`: Percona XtraDB Cluster Operator Pod crashing on missing HAProxy PodDisruptionBudget (Thanks to user indiebrain for contribution)
* :jirabug:`K8SPXC-372`: TLS - wrong "apiGroups" name for cert-manager (recent versions) blocking issuer creation (Thanks to user rf_enigm for reporting this issue)
* :jirabug:`K8SPXC-355`: Counterintuitive YYYY-DD-MM dates in the S3 backup folder names (Thanks to user graham.webcurl for reporting this issue)
* :jirabug:`K8SPXC-274`: Upgrade path from 1.2.0 -> 1.3.0 -> 1.4.0 not working (Thanks to user martin.atroo for reporting this issue)
* :jirabug:`K8SPXC-426`: mysqld recovery logs not logged to file and not available through ``kubectl logs``
* :jirabug:`K8SPXC-419`: Percona XtraDB Cluster incremental state transfers not taken into account by readiness/liveness checks
* :jirabug:`K8SPXC-418`: HA Proxy not routing traffic for 1 donor, 2 joiners
* :jirabug:`K8SPXC-417`: Certmanager not compatible with Kubernetes versions below v1.15 due to unnecessarily high API version demand
* :jirabug:`K8SPXC-364`: Smart Updates showing empty "from" versions for non-PXC objects in logs
* :jirabug:`K8SPXC-311`: Failed backups having "Running" status for indefinite time
* :jirabug:`K8SPXC-400`: **(controversial ticket)** haproxy should not create pvc's
* :jirabug:`K8SPXC-379`: operator user credentials not added into internal secrets on upgrade from 1.4.0 (Thanks to user pservit for reporting this issue)
* :jirabug:`K8SPXC-371`: PXC debug images not reacting on failed recovery attempt due to no sleep after the mysqld exit
