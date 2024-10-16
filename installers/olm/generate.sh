#!/usr/bin/env bash
set -eu

DISTRIBUTION="$1"

cd "${BASH_SOURCE[0]%/*}"

bundle_directory="bundles/${DISTRIBUTION}"
project_directory="projects/${DISTRIBUTION}"
go_api_directory=$(cd ../../pkg/apis && pwd)

# The 'operators.operatorframework.io.bundle.package.v1' package name for each
# bundle (updated for the 'certified' and 'marketplace' bundles).
package_name='percona-xtradb-cluster-operator'

# The project name used by operator-sdk for initial bundle generation.
project_name='percona-xtradb-cluster-operator'

# The prefix for the 'clusterserviceversion.yaml' file.
# Per OLM guidance, the filename for the clusterserviceversion.yaml must be prefixed
# with the Operator's package name for the 'redhat' and 'marketplace' bundles.
# https://github.com/redhat-openshift-ecosystem/certification-releases/blob/main/4.9/ga/troubleshooting.md#get-supported-versions
file_name='percona-xtradb-cluster-operator'

kubectl kustomize "../../config/${DISTRIBUTION}" >operator_yamls.yaml

yq eval '. | select(.kind == "CustomResourceDefinition")' operator_yamls.yaml >operator_crds.yaml
yq eval '. | select(.kind == "Deployment")' operator_yamls.yaml >operator_deployments.yaml
yq eval '. | select(.kind == "ServiceAccount")' operator_yamls.yaml >operator_accounts.yaml
yq eval '. | select(.kind == "Role")' operator_yamls.yaml >operator_roles.yaml

## Recreate the Operator SDK project.

[ ! -d "${project_directory}" ] || rm -r "${project_directory}"
install -d "${project_directory}"
(
	cd "${project_directory}"
	operator-sdk init --fetch-deps='false' --project-name=${project_name}

	# Generate CRD descriptions from Go markers.
	# https://sdk.operatorframework.io/docs/building-operators/golang/references/markers/
	yq eval '[. | {"group": .spec.group, "kind": .spec.names.kind, "version": .spec.versions[].name}]' ../../operator_crds.yaml >crd_gvks.yaml

	yq eval --inplace '.multigroup = true | .resources = load("crd_gvks.yaml" | fromyaml) | .' ./PROJECT

	ln -s "${go_api_directory}" .
	operator-sdk generate kustomize manifests --interactive='false' --verbose
)

# Recreate the OLM bundle.
[ ! -d "${bundle_directory}" ] || rm -r "${bundle_directory}"
install -d \
	"${bundle_directory}/manifests" \
	"${bundle_directory}/metadata"

# `echo "${operator_yamls}" | operator-sdk generate bundle` includes the ServiceAccount which cannot
# be upgraded: https://github.com/operator-framework/operator-lifecycle-manager/issues/2193

# Render bundle annotations and strip comments.
# Per Red Hat we should not include the org.opencontainers annotations in the
# 'redhat' & 'marketplace' annotations.yaml file, so only add them for 'community'.
# - https://coreos.slack.com/team/UP1LZCC1Y

export package="${package_name}"
export package_channel="${PACKAGE_CHANNEL}"
export openshift_supported_versions="${OPENSHIFT_VERSIONS}"

yq eval '.annotations["operators.operatorframework.io.bundle.channels.v1"] = $package_channel |
         .annotations["operators.operatorframework.io.bundle.channel.default.v1"] = $package_channel |
         .annotations["com.redhat.openshift.versions"] = env(openshift_supported_versions)' \
	bundle.annotations.yaml >"${bundle_directory}/metadata/annotations.yaml"

if [ ${DISTRIBUTION} == 'community' ]; then
	# community-operators
	yq eval '.annotations["operators.operatorframework.io.bundle.package.v1"] = "percona-xtradb-cluster-operator" |
         .annotations["org.opencontainers.image.authors"] = "info@percona.com" |
         .annotations["org.opencontainers.image.url"] = "https://percona.com" |
         .annotations["org.opencontainers.image.vendor"] = "Percona"' \
		bundle.annotations.yaml >"${bundle_directory}/metadata/annotations.yaml"

# certified-operators
elif [ ${DISTRIBUTION} == 'redhat' ]; then
	yq eval --inplace '
    .annotations["operators.operatorframework.io.bundle.package.v1"] = "percona-xtradb-cluster-operator-certified" ' \
		"${bundle_directory}/metadata/annotations.yaml"

# redhat-marketplace
elif [ ${DISTRIBUTION} == 'marketplace' ]; then
	yq eval --inplace '
    .annotations["operators.operatorframework.io.bundle.package.v1"] = "percona-xtradb-cluster-operator-certified-rhmp" ' \
		"${bundle_directory}/metadata/annotations.yaml"
fi

# Copy annotations into Dockerfile LABELs.
# TODO fix tab for labels.

labels=$(yq eval -r '.annotations | to_entries | map("    " + .key + "=" + (.value | tojson)) | join("\n")' \
	"${bundle_directory}/metadata/annotations.yaml")

ANNOTATIONS="${labels}" envsubst <bundle.Dockerfile >"${bundle_directory}/Dockerfile"

# Include CRDs as manifests.
crd_names=$(yq eval -o=tsv '.metadata.name' operator_crds.yaml)

for name in ${crd_names}; do
	yq eval ". | select(.metadata.name == \"${name}\")" operator_crds.yaml >"${bundle_directory}/manifests/${name}.crd.yaml"
done

abort() {
	echo >&2 "$@"
	exit 1
}
dump() { yq --color-output; }

# The first command render yaml correctly and the second extract data.

yq eval -i '[.]' operator_deployments.yaml && yq eval 'length == 1' operator_deployments.yaml --exit-status >/dev/null || abort "too many deployments accounts!" $'\n'"$(yq eval . operator_deployments.yaml)"

yq eval -i '[.]' operator_accounts.yaml && yq eval 'length == 1' operator_accounts.yaml --exit-status >/dev/null || abort "too many service accounts!" $'\n'"$(yq eval . operator_accounts.yaml)"

yq eval -i '[.]' operator_roles.yaml && yq eval 'length == 1' operator_roles.yaml --exit-status >/dev/null || abort "too many roles!" $'\n'"$(yq eval . operator_roles.yaml)"

# Render bundle CSV and strip comments.
csv_stem=$(yq -r '.projectName' "${project_directory}/PROJECT")

cr_example=$(yq eval -o=json '[.]' ../../deploy/cr.yaml)

export examples="${cr_example}"
export deployment=$(yq eval operator_deployments.yaml)
export account=$(yq eval '.[] | .metadata.name' operator_accounts.yaml)
export rules=$(yq eval '.[] | .rules' operator_roles.yaml)
export version="${VERSION}"
export minKubeVer="${MIN_KUBE_VERSION}"
export stem="${csv_stem}"
export timestamp=$(date -u +"%Y-%m-%dT%H:%M:%S.%3Z")
export name="${csv_stem}.v${VERSION}"
export name_certified="${csv_stem}-certified.v${VERSION}"
export name_certified_rhmp="${csv_stem}-certified-rhmp.v${VERSION}"
export skip_range="<${VERSION}"
export containerImage=$(yq eval '.[0].spec.template.spec.containers[1].image' operator_deployments.yaml)
export relatedImages=$(yq eval bundle.relatedImages.yaml)
relIm==$(yq eval bundle.relatedImages.yaml)

yq eval '
  .metadata.annotations["alm-examples"] = strenv(examples) |
  .metadata.annotations["containerImage"] = env(containerImage) |
  .metadata.annotations["olm.skipRange"] = env(skip_range) |
  .metadata.annotations["createdAt"] = env(timestamp) |
  .metadata.name = env(name) |
  .spec.version = env(version) |
  .spec.install.spec.permissions = [{ "serviceAccountName": env(account), "rules": env(rules) }] |
  .spec.install.spec.deployments = [( env(deployment) | .[] |{ "name": .metadata.name, "spec": .spec} )] |
  .spec.minKubeVersion = env(minKubeVer)' bundle.csv.yaml >"${bundle_directory}/manifests/${file_name}.clusterserviceversion.yaml"

if [[ ${DISTRIBUTION} == "redhat" ]]; then
	echo "REDHAT"
	yq eval --inplace '
        .spec.relatedImages = env(relatedImages) |
        .metadata.annotations.certified = "true" |
        .metadata.annotations["containerImage"] = "registry.connect.redhat.com/percona/percona-xtradb-cluster-operator@sha256:<update_operator_SHA_value>" |
        .metadata.name = strenv(name_certified)' \
		"${bundle_directory}/manifests/${file_name}.clusterserviceversion.yaml"

elif [[ ${DISTRIBUTION} == "marketplace" ]]; then
	# Annotations needed when targeting Red Hat Marketplace
	export package_url="https://marketplace.redhat.com/en-us/operators/${file_name}"
	yq --inplace '
        .metadata.name = env(name_certified_rhmp) |
        .metadata.annotations["containerImage"] = "registry.connect.redhat.com/percona/percona-xtradb-cluster-operator@sha256:<update_operator_SHA_value>" |
        .metadata.annotations["marketplace.openshift.io/remote-workflow"] =
            "https://marketplace.redhat.com/en-us/operators/percona-xtradb-cluster-operator-certified-rhmp/pricing?utm_source=openshift_console" |
        .metadata.annotations["marketplace.openshift.io/support-workflow"] =
            "https://marketplace.redhat.com/en-us/operators/percona-xtradb-cluster-operator-certified-rhmp/support?utm_source=openshift_console" |
        .spec.relatedImages = env(relatedImages)' \
		"${bundle_directory}/manifests/${file_name}.clusterserviceversion.yaml"
fi

if >/dev/null command -v tree; then tree -C "${bundle_directory}"; fi