.. _K8SPXC-1.6.0:qaq

================================================================================
*Percona Kubernetes Operator for Percona XtraDB Cluster* 1.6.0
================================================================================

:Date: September 30, 2020
:Installation: `Installing Percona Kubernetes Operator for Percona XtraDB Cluster <https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html#quickstart-guides>`_

New Features
================================================================================

* :jirabug:`K8SPXC-429`: Possibility to restore backups to a new Kubernetes-based environment
* :jirabug:`K8SPXC-368`: Auto update system users by changing the appropriate secret name
* :jirabug:`K8SPXC-144`: Allow adding ProxySQL configuration options
* :jirabug:`K8SPXC-343`: Helm chart officially provided with the Operator

Improvements
================================================================================

* :jirabug:`K8SPXC-398`: New crVersion key in ``deploy/cr.yaml`` to indicate the API version that the Custom Resource corresponds to
* :jirabug:`K8SPXC-416`: Support of the proxy-protocol in HAProxy
* :jirabug:`K8SPXC-372`: Support new versions of cert-manager by the Operator (Thanks to user rf_enigm for reporting this issue)
* :jirabug:`K8SPXC-317`: The possibility to configure the Operator's imagePullPolicy option (Thanks to user imranrazakhan for reporting this issue)
* :jirabug:`K8SPXC-438`: Cluster name length limit extended to 32 characters to fit the maximum value allowed by wsrep_cluster_name
* :jirabug:`K8SPXC-411`: Extend certmanager configuration to add more domains (multiple SAN) to certificate

Bugs Fixed
================================================================================

* :jirabug:`K8SPXC-431`: HAProxy unable to start on OpenShift with the default ``cr.yaml`` file
* :jirabug:`K8SPXC-408`: Insufficient MAX_USER_CONNECTIONS=10 for ProxySQL monitor user (increased to 100)
* :jirabug:`K8SPXC-391`: HA Proxy and PMM cannot be enabled at the same time (Thanks to user rf_enigm for reporting this issue)
* :jirabug:`K8SPXC-406`: Second node (XXX-pxc-1) always selected as donor (Thanks to user pservit for reporting this issue)
* :jirabug:`K8SPXC-390`: Crash on missing HAProxy PodDisruptionBudget
* :jirabug:`K8SPXC-355`: Counterintuitive YYYY-DD-MM dates in the S3 backup folder names (Thanks to user graham.webcurl for reporting this issue)
* :jirabug:`K8SPXC-274`: The 1.2.0 -> 1.3.0 -> 1.4.0 upgrade path not working (Thanks to user martin.atroo for reporting this issue)
* :jirabug:`K8SPXC-450`: The unnecessary ssls annotations added to HA Proxy Pods are potential reasons of the Pod restart if changed
* :jirabug:`K8SPXC-443`: The outdated version service endpoint URL
* :jirabug:`K8SPXC-435`: MySQL root password visible through ``kubectl logs``
* :jirabug:`K8SPXC-426`: mysqld recovery logs not logged to file and not available through ``kubectl logs``
* :jirabug:`K8SPXC-423`: HAProxy not refreshing IP addresses even when the node gets different address
* :jirabug:`K8SPXC-419`: Percona XtraDB Cluster incremental state transfers not taken into account by readiness/liveness checks
* :jirabug:`K8SPXC-418`: HAProxy not routing traffic for 1 donor, 2 joiners
* :jirabug:`K8SPXC-417`: Cert-manager not compatible with Kubernetes versions below v1.15 due to unnecessarily high API version demand
* :jirabug:`K8SPXC-364`: Smart Updates showing empty "from" versions for non-PXC objects in logs
* :jirabug:`K8SPXC-379`: operator user credentials not added into internal secrets on upgrade from 1.4.0 (Thanks to user pservit for reporting this issue)
* :jirabug:`K8SPXC-371`: PXC debug images not reacting on failed recovery attempt due to no sleep after the mysqld exit
