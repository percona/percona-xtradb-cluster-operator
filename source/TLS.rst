TLS
===

Security is configured at multiple levels. Transport Layer Security
(TLS) secures the API endpoints and is a
cryptographic protocol. TLS provides secure communications over a computer
network.

We recommend that you run your cluster with the TLS protocol enabled. For demonstration purposes, you can disable the TLS protocol by editing cr.yaml/pxc/allowUnstafeConfigurations and changing the value to `true`. Be sure to reset the value when you have completed your tasks.


Certificate Authority
=====================
The Certificate Authority issues the certificates.
You can generate certificates with the following steps:
    1. Provision a Certificate Authority (CA) to generate TLS certificates
    2. Generate a CA key and certificate file with the server details
    3. Create the server TLS certificates using the CA keys, certs, and server details

  The command generates certificate files with the following attributes:
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

You define a Certificate Authority certificate, which is called the root certificate. From the command line, use base64 to encode the key and certificate values. Run the following statements::

  cat tls.crt | base64
  cat tls.key | base64

Copy and paste each encoded value into the appropriate sections of the YAML file. Each value is one line.

You can use then use the YAML file to create the secret::

  kubectl create -f secret-ssl.yaml
