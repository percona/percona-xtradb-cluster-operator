# Percona XtraDB Cluster Operator

A Kubernetes operator for [Percona XtraDB Cluster](https://www.percona.com/software/mysql-database/percona-xtradb-cluster) based on the [Operator SDK](https://github.com/operator-framework/operator-sdk).

# :heavy_exclamation_mark: In the stage of active developing. Not ready for use yet.

## Run
```sh
kubectl create -f deploy/rbac.yaml
kubectl create -f deploy/crd.yaml
kubectl apply -f deploy/cr.yaml
```

```sh
OPERATOR_NAME=<operator-name> operator-sdk up local --namespace=<namespace>
```

## Delete
```sh
kubectl delete -f deploy/crd.yaml
kubectl delete -f deploy/rbac.yaml
kubectl delete -f deploy/cr.yaml
```