Transport Layer Security (TLS)
******************************

Security is configured at multiple levels. Transport Layer Security
(TLS) secures the API endpoints and is a
cryptographic protocol. TLS provides secure communications over a computer
network.

The Percona Kubernetes Operator for PXC uses a cert-manager and supports manual configuration, which is available for all versions of K8s and is an upstream feature. A cert-manager is a Kubernetes tool widely used for to automate the management and issuance of TLS certificates.


Install the cert-manager
========================


The cert-manager is community-driven, and open source. The steps to install the cert-manager are the following:

  * Create a namespace
  * Disable resource validations on the cert-manager namespace
  * Install the cert-manager.

The following commands perform the needed actions:

::

    kubectl label namespace cert-manager certmanager.k8s.io/disable-validation=true
    kubectl apply -f https://raw.githubusercontent.com/jetstack/cert-manager/release-0.7/deploy/manifests/cert-manager.yaml

After the installation, you can verify the cert-manager by running the following command:

::

  kubectl get pods <cert-manager namespace>

The result displays the cert-manager and webhook active and running.

Run PXC with auto-generated certificates
========================================


When you deploy the operator, the operator creates a self-signed issuer. The self-signed issuer is a Certificate Authority (CA) and generates certificates. The Percona Operator self-signed issuer is local to the operator namespace. This self-signed issuer is created because PXC requires all certrificates are issued by the same CA.

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

A disadvantage of generating certificates with the command line can make managing the infrastructure difficult to manage, document, and reproduce. You can use a YAML file to maintain your key and certificate data and save the file to a secure location.

The structure of the Secret-SSL.yaml file is::

  apiVersion: v1
    kind: Secret
    metadata:
      name: <your cluster name>
    type: kubernetes.io/tls
    data:
      ca.crt: <encoded value>
      tls.crt: <encoded value>
      tls.key: <encoded value>

You define a Certificate Authority certificate, which is called the root certificate. From the command line, use base64 to encode the key and certificate values. Run the following commands::

  echo tls.crt | base64
  echo tls.key | base64

Copy and paste each encoded value into the appropriate sections of the YAML file. Each value is one line.

You can use then use the YAML file to create the secret::

  kubectl create -f secret-ssl.yaml

Run PXC without TLS
==========================


We recommend that you run your cluster with the TLS protocol enabled. For demonstration purposes, disable the TLS protocol by editing the `cr.yaml/spec/pxc/allowUnstafeConfigurations` setting to `true`. Be sure to reset the value when you have completed your tasks.
