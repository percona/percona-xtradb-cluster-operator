.. _K8SPXC-1.4.0:

================================================================================
*Percona Kubernetes Operator for PXC* 1.4.0
================================================================================

:Date: March XX, 2020

:Installation: `Installing Percona Kubernetes Operator for PXC <https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html#installation>`_

New Features
================================================================================

* :jirabug:`K8SPXC-125`: Percona XtraDB Cluster 8.0 is now supported
* :jirabug:`K8SPXC-95`: Amazon Elastic Container Service for Kubernetes (EKS)
  was added to the list of the officially supported platforms
* The OpenShift Container Platform 4.3 is now supported.
* :jirabug:`K8SPXC-172`: Full data-at-rest encryption available in PXC 8.0 is now supported by the Operator. This feature is implemented with the help of the ``keyring_vault`` plugin which ships with PXC 8.0.  By utilizing `Vault <https://www.vaultproject.io>`_ we enable our customers to follow best practices with encryption in their environment.

Improvements
================================================================================

* :jirabug:`K8SPXC-221`: Operator now updates observedGeneration status message to allow better monitoring of the cluster rollout or backup/restore process
* :jirabug:`K8SPXC-213`: A special :ref:`PXC debug image<debug-images>` is now available. It avoids restarting on fail and contains additional tools useful for debugging
* :jirabug:`K8SPXC-100`: The Operator now implements the crash tolerance on the one member crash. The implementation is based on starting Pods with ``mysqld --wsrep_recover`` command if there was no graceful shutdown
* :jirabug:`K8SPXC-262`: The Operator allows setting ephemeral-storage requests and limits on all Pods

Bugs Fixed
================================================================================

* :jirabug:`K8SPXC-153`: S3 protocol credentials were not masked in logs during the PXC backup & restore process
* :jirabug:`K8SPXC-222`: The Operator got caught in reconciliation error in case of the erroneous/absent API version in the ``deploy/cr.yaml`` file
* :jirabug:`K8SPXC-220`: The inability to update or delete existing CRD was possible because of too large records in etcd, resulting in “request is too large” errors. Only 20 last status changes are now stored in etcd to avoid this problem.
* :jirabug:`K8SPXC-52`: The Operator produced an unclear error message in case of fail caused by the absent or malformed pxc section in the ``deploy/cr.yaml`` file
* :jirabug:`K8SPXC-219`: PXC Helm charts were incompatible with the version 3 of the Helm package manager
* :jirabug:`K8SPXC-40`: The cluster was unable to reach "ready" status in case if ``ProxySQL.Enabled`` field was set to ``false``
* :jirabug:`K8SPXC-34`: Change of the ``proxysql.servicetype`` filed was not detected by the Operator and thus had no effect
* :jirabug:`K8SPXC-261`: proxysql logs were showing the root password
* :jirabug:`K8SPXC-263`: The incorrect endpoint was shown by the the ``kubectl get pxc`` command

Help us improve our software quality by reporting any bugs you encounter using
`our bug tracking system <https://jira.percona.com/secure/Dashboard.jspa>`_.
