# Percona XtraDB Cluster Operator RedHat CSV certification

RedHat requires a CSV bundle compiled into a specific docker image.
More about that you can find [here](https://redhat-connect.gitbook.io/certified-operator-guide/appendix/bundle-maintenance-after-migration)

Please pay attention to the following:
- X.X.X/ should contain only metadata/ and manifests/. Within those directories you should only have yaml files.

- Technically you can remove all of the yaml files from within the 1.6.0/ directory as well as the packagel.yaml. As the Bundle image when built ignores those anyway. The COPY lines from your Dockerfile only pull the metadata/ and manifests/ files and there is a LABEL in the dockerfile that handles the package.yaml. (LABEL operators.operatorframework.io.bundle.package.v1=percona-xtradb-cluster-operator-certified)

## Release
In order to deliver package to RedHat you need to login to docker [registry](https://connect.redhat.com/project/5878691/images/upload-image) and execute:

```bash
export TAG=X.X.X
docker build . -f ./bundle-${TAG}.Dockerfile -t scan.connect.redhat.com/ospid-9e82dc93-2571-41bf-a5dc-722848051cbf/percona-xtradb-cluster-operator-certified-bundle:${TAG}
docker push scan.connect.redhat.com/ospid-9e82dc93-2571-41bf-a5dc-722848051cbf/percona-xtradb-cluster-operator-certified-bundle:${TAG}
```
