1. To generate bundle correctly please set env variables (default values for these variables you can check in makefile):
```bash
# operator version
export VERSION=1.18.0
# By default we use perconalab for tag owner. Please update this variable to use another repo
export IMAGE_TAG_OWNER=percona
# Min k8s version
export MIN_KUBE_VERSION=1.27.0
# Openshift versions:
export OPENSHIFT_VERSIONS="v4.13-v4.16"
# Set namespace or cluster (to generate bundles for cluster-wide)
export MODE=namespace
```
2. Also it could be useful to check variable in makefile and update if you need something extra. For the most cases to update these variables is enough
3. Update spec.description in bundle.csv.yaml with features added in this release.
4. Run bundle generation:
```bash
# Generate all bundles community redhat and marketplace:
make bundles
# Generate only specific bundle:
make bundles/community
```