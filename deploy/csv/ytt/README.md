# Cluster Service Version generation

Since we got bored with manual work on CSV generation, I'd like to infroduce you the automated way
how we can make our lives easier.
You need to install:

* [ytt](https://carvel.dev/ytt/docs/latest/install/)
* [yq](https://mikefarah.gitbook.io/yq/v/v4.x#install)
and then run

```bash
export SRC_PATH="$(git rev-parse --show-toplevel)"
export CLUSTER_VERSION="$(yq e '.spec.crVersion' ${SRC_PATH}/deploy/cr.yaml)"
export OPERATOR_NAME="$(yq e '.metadata.name' ${SRC_PATH}/deploy/operator.yaml)"
export OUTPUT_PATH="/tmp/${CLUSTER_VERSION}/manifests"
pushd ${SRC_PATH}/deploy/csv/ytt
    yq e -s '.metadata.name' ${SRC_PATH}/deploy/crd.yaml
    for crd in *.percona.com.yml; do mv ${crd} ${crd//yml/crd.yaml}; done
popd
ytt -f ${SRC_PATH}/deploy/csv/ytt \
    --data-value-file manifest.operator="${SRC_PATH}/deploy/operator.yaml" \
    --data-value-file manifest.cr="${SRC_PATH}/deploy/cr.yaml" \
    --data-value-file manifest.restore="${SRC_PATH}/deploy/backup/restore.yaml" \
    --data-value-file manifest.backup="${SRC_PATH}/deploy/backup/backup.yaml" \
    --data-value-file manifest.secrets="${SRC_PATH}/deploy/secrets.yaml" \
    --data-value-file rn_txt="${SRC_PATH}/deploy/csv/ytt/RN.md" \
    --data-value manifest.rbac="$(yq eval 'select(documentIndex == 0)' ${SRC_PATH}/deploy/rbac.yaml)" \
    --data-value last_version="${CLUSTER_VERSION}" \
    --data-value platform="redhat" \
    --output-files "${OUTPUT_PATH}" \
    --file-mark "csv.yaml:path=${OPERATOR_NAME}.v${CLUSTER_VERSION}.clusterserviceversion.yaml"
```
