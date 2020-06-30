# Percona XtraDB Cluster Operator

A Kubernetes operator for [Percona XtraDB Cluster](https://www.percona.com/software/mysql-database/percona-xtradb-cluster) based on the [Operator SDK](https://github.com/operator-framework/operator-sdk).

# Documentation
See the [Official Documentation](https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html) for more information.

[![Official Documentation](https://via.placeholder.com/260x60/419bdc/FFFFFF/?text=Documentation)](https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html)

## How to deploy

Create custom resource definitions required

```bash
kubectl apply -f https://raw.githubusercontent.com/percona/percona-xtradb-cluster-operator/aws-v1.4.0/deploy/crd.yaml
```

Create the corresponding RBAC policies

```bash
kubectl apply -f https://raw.githubusercontent.com/percona/percona-xtradb-cluster-operator/aws-v1.4.0/deploy/rbac.yaml
```

Create or update database secrets (user passwords)

```bash
kubectl apply -f https://raw.githubusercontent.com/percona/percona-xtradb-cluster-operator/aws-v1.4.0/deploy/secrets.yaml
```

Deploy operator into K8S cluster

```bash
kubectl apply -f https://raw.githubusercontent.com/percona/percona-xtradb-cluster-operator/aws-v1.4.0/deploy/operator.yaml
```



