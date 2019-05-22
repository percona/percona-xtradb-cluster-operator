Transport Layer Security (TLS)
******************************

Security is configured at multiple levels. Transport Layer Security
(TLS) is a
cryptographic protocol. TLS provides secure communications over a computer
network.

The Percona Kubernetes Operator for PXC uses a cert-manager and supports manual configuration, which is available for all versions of Kubernetes and is an upstream feature. A cert-manager is a Kubernetes tool widely used to automate the management and issuance of TLS certificates.

The Percona Kubernetes Operator for PXC requires TLS for the following types of communication:
  * Internal - communication between PXC instances in the cluster
  * External - communication between the client application and ProxySQL

The internal certificate is also an authorization method.

Install the cert-manager
========================


The cert-manager is community-driven, and open source. The steps to install the cert-manager are the following:

  * Create a namespace
  * Disable resource validations on the cert-manager namespace
  * Install the cert-manager.

The following commands perform the needed actions:

::

    kubectl label namespace cert-manager certmanager.k8s.io/disable-validation=true
    kubectl create namespace cert-manager
     kubectl label namespace cert-manager certmanager.k8s.io/disable-validation=true
    kubectl apply -f https://raw.githubusercontent.com/jetstack/cert-manager/release-0.7/deploy/manifests/cert-manager.yaml

After the installation, you can verify the cert-manager by running the following command:

::

  kubectl get pods <cert-manager namespace>
  kubectl get pods -n cert-manager

The result displays the cert-manager and webhook active and running.

When you deploy the operator, the operator requests a certificate from the  cert-manager. The cert-manager is a self-signed issuer and generates certificates. The Percona Operator self-signed issuer is local to the operator namespace. This self-signed issuer is created because PXC requires all certificates are issued by the same CA.

The creation of the self-signed issuer allows you to deploy and use the Percona Operator without creating a clusterissuer separately.


Generate certificates manually
==============================

To generate certificates follow these steps:

  1. Provision a Certificate Authority (CA) to generate TLS certificates
  2. Generate a CA key and certificate file with the server details
  3. Create the server TLS certificates using the CA keys, certs, and server details


The set of commands generate certificates with the following attributes:

  *  Server-pem - Certificate
  *  Server-key.pem - the private key
  *  ca.pem - Certificate Authority

A created secret must be added to cr.yaml/spec/secretsName. A certificate generated for internal communications must be added to the cr.yaml/spec/sslInternalSecretName.

::

  cat <<EOF | cfssl gencert -initca - | cfssljson -bare ca
  {
    "CN": "Root CA",
    "key": {
      "algo": "rsa",
      "size": 2048
    }
  }
  EOF

  cat <<EOF | cfssl gencert -ca=ca.pem  -ca-key=ca-key.pem - | cfssljson -bare server
  {
    "hosts": [
      "${CLUSTER_NAME}-proxysql",
      "*.${CLUSTER_NAME}-proxysql-unready",
      "*.${CLUSTER_NAME}-pxc"
    ],
    "CN": "${CLUSTER_NAME}-pxc",
    "key": {
      "algo": "rsa",
      "size": 2048
    }
  }
  EOF

  kubectl create secret generic my-cluster-ssl --from-file=tls.crt=server.pem --
  from-file=tls.key=server-key.pem --from-file=ca.crt=ca.pem --
  type=kubernetes.io/tls


You can use the YAML file to create the secret::

  kubectl create -f secret-ssl.yaml

Run PXC without TLS
==========================

We recommend that you run your cluster with the TLS protocol enabled. For demonstration purposes, disable the TLS protocol by editing the `cr.yaml/spec/pxc/allowUnstafeConfigurations` setting to `true`.
