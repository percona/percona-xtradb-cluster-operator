TLS
===

Security is configured at multiple levels. Transport Layer Security
(TLS) secures the API endpoints. Transport Layer Security (TLS) is a
cryptographic protocol. TLS provides communications over a computer
network. A certificate includes a key, information, and the certificate
verifies the certificate’s contents. The certificate issuer is a
certificate authority and issues certificate signing request (CSR).

The tool, cert-manager, automates the management and issuance of TLS
certificates.

To verify if the certificate is signed and approved, run the following
command:

.. code:: bash

   kubectl get csr

The response lists the CSR available.

::

   NAME                                                   AGE       REQUESTOR   CONDITION
   node-csr-3QdRRgDlrFQUS6Osj1z9c7YVuHmjIskNO-TgUHCrUCQ   18m       kubelet     Approved,Issued
   node-csr-AYpnA2VSU3Kcwzwd0FL5eicEV9GyzzOWUqaR3yzz1-8   18m       kubelet     Approved,Issued
   node-csr-OYCU38VyugC4C0QCHLoudvgkSKITg1yT52ovaRlAcnE   18m       kubelet     Approved,Issued

You can list the secrets created for the cluster:

.. code:: bash

   kubectl get secrets

The response lists the available secrets:

::

   NAME                  TYPE                                  DATA      AGE
   default-token-k59r7   kubernetes.io/service-account-token   3         7m
   my-cluster-secrets    Opaque                                6         5m
