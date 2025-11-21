#!/usr/bin/env bash

# Install
# brew install gawk coreutils
for command in gawk gcsplit; do
	if ! command -v $command &>/dev/null; then
		echo "Error: $command is not installed. Please install it: brew install $command" >&2
		exit 1
	fi
done

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
NS_RESOURCE_RBAC="../rbac/namespace"
CLUSTER_RESOURCE_RBAC="../rbac/cluster"
NS_RESOURCE_OPERATOR="../manager/namespace"
CLUSTER_RESOURCE_OPERATOR="../manager/cluster"
KUSTOMIZATION_FILE="../../config/bundle/kustomization.yaml"

if [ "${MODE}" == "cluster" ]; then
	suffix="-cw"
	mode="Cluster"
	rulesLevel="ClusterPermissions"
	sed -i '' "s|$NS_RESOURCE_RBAC|$CLUSTER_RESOURCE_RBAC|g" "$KUSTOMIZATION_FILE"
	sed -i '' "s|$NS_RESOURCE_OPERATOR|$CLUSTER_RESOURCE_OPERATOR|g" "$KUSTOMIZATION_FILE"
elif [ "${MODE}" == "namespace" ]; then
	suffix=""
	mode=""
	rulesLevel="permissions"
	sed -i '' "s|$CLUSTER_RESOURCE_RBAC|$NS_RESOURCE_RBAC|g" "$KUSTOMIZATION_FILE"
	sed -i '' "s|$CLUSTER_RESOURCE_OPERATOR|$NS_RESOURCE_OPERATOR|g" "$KUSTOMIZATION_FILE"
else
	echo "Please add MODE variable. It could be either namespace or cluster"
	exit 1
fi
# Copy operator file to config:
cp ../../deploy/operator.yaml ../../config/manager/namespace
cp ../../deploy/cw-operator.yaml ../../config/manager/cluster

# Copy RBAC:
gcsplit --elide-empty-files -f output- ../../deploy/rbac.yaml "/^---$/" "{*}"
target_dir="../../config/rbac/namespace"
mv output-00 "$target_dir/role.yaml"
mv output-01 "$target_dir/service_account.yaml"
mv output-02 "$target_dir/role_binding.yaml"

# Copy RBAC for CW:
gcsplit --elide-empty-files -f output- ../../deploy/cw-rbac.yaml "/^---$/" "{*}"
target_dir="../../config/rbac/cluster"
mv output-00 "$target_dir/role.yaml"
mv output-01 "$target_dir/service_account.yaml"
mv output-02 "$target_dir/role_binding.yaml"

kubectl kustomize "../../config/${DISTRIBUTION}" >operator_yamls.yaml

export role="${mode}Role"
yq eval '. | select(.kind == "CustomResourceDefinition")' operator_yamls.yaml >operator_crds.yaml
yq eval '. | select(.kind == "Deployment")' operator_yamls.yaml >operator_deployments.yaml
yq eval '. | select(.kind == "ServiceAccount")' operator_yamls.yaml >operator_accounts.yaml
yq eval '. | select(.kind == env(role))' operator_yamls.yaml >operator_roles${suffix}.yaml

update_yaml_images() {
	local yaml_file="$1"

	if [ ! -f "$yaml_file" ]; then
		echo "Error: File '$yaml_file' does not exist."
		return 1
	fi

    local temp_file
    temp_file=$(mktemp)

	sed -E 's/(("image":|containerImage:|image:)[ ]*"?)([^"]+)("?)/\1docker.io\/\3\4/g' "$yaml_file" >"$temp_file"
	mv "$temp_file" "$yaml_file"

	echo "File '$yaml_file' updated successfully."
}

## Recreate the Operator SDK project.

[ ! -d "${project_directory}" ] || rm -r "${project_directory}"
install -d "${project_directory}"
(
	cd "${project_directory}"
	operator-sdk init --fetch-deps='false' --project-name=${project_name}

	# Generate CRD descriptions from Go markers.
	# https://sdk.operatorframework.io/docs/building-operators/golang/references/markers/
	yq eval '[. | {"group": .spec.group, "kind": .spec.names.kind, "version": .spec.versions[].name}]' ../../../../deploy/crd.yaml >crd_gvks.yaml

	yq eval --inplace '.multigroup = true | .resources = load("crd_gvks.yaml" | fromyaml) | .' ./PROJECT

	ln -s "${go_api_directory}" .
	operator-sdk generate kustomize manifests --interactive='false' --verbose
)

# Recreate the OLM bundle.
[ ! -d "${bundle_directory}" ] || rm -r "${bundle_directory}"
install -d \
	"${bundle_directory}/manifests" \
	"${bundle_directory}/metadata"

# Render bundle annotations and strip comments.
# Per Red Hat we should not include the org.opencontainers annotations in the
# 'redhat' & 'marketplace' annotations.yaml file, so only add them for 'community'.
# - https://coreos.slack.com/team/UP1LZCC1Y

export package="${package_name}"
export package_channel="${PACKAGE_CHANNEL}${suffix}"
export openshift_supported_versions="${OPENSHIFT_VERSIONS}"

yq eval '.annotations["operators.operatorframework.io.bundle.channels.v1"] = env(package_channel) |
         .annotations["operators.operatorframework.io.bundle.channel.default.v1"] = env(package_channel) |
         .annotations["com.redhat.openshift.versions"] = env(openshift_supported_versions)' \
	bundle.annotations.yaml >"${bundle_directory}/metadata/annotations.yaml"

if [ "${DISTRIBUTION}" == 'community' ]; then
	# community-operators
	yq eval --inplace '
	.annotations["operators.operatorframework.io.bundle.package.v1"] = "percona-xtradb-cluster-operator" |
    .annotations["org.opencontainers.image.authors"] = "info@percona.com" |
    .annotations["org.opencontainers.image.url"] = "https://percona.com" |
     .annotations["org.opencontainers.image.vendor"] = "Percona"' \
		"${bundle_directory}/metadata/annotations.yaml"

# certified-operators
elif [ "${DISTRIBUTION}" == 'redhat' ]; then
	yq eval --inplace '
    .annotations["operators.operatorframework.io.bundle.package.v1"] = "percona-xtradb-cluster-operator-certified" ' \
		"${bundle_directory}/metadata/annotations.yaml"

# redhat-marketplace
elif [ "${DISTRIBUTION}" == 'marketplace' ]; then
	yq eval --inplace '
    .annotations["operators.operatorframework.io.bundle.package.v1"] = "percona-xtradb-cluster-operator-certified-rhmp" ' \
		"${bundle_directory}/metadata/annotations.yaml"
fi

# Copy annotations into Dockerfile LABELs.
# TODO fix tab for labels.

labels=$(yq eval -r '.annotations | to_entries | map("LABEL " + .key + "=" + (.value | tojson)) | join("\n")' \
	"${bundle_directory}/metadata/annotations.yaml")

labels="${labels}
LABEL com.redhat.delivery.backport=true
LABEL com.redhat.delivery.operator.bundle=true"


LABELS="${labels}" envsubst <bundle.Dockerfile >"${bundle_directory}/Dockerfile"

awk '{gsub(/^[ \t]+/, "    "); print}' "${bundle_directory}/Dockerfile" >"${bundle_directory}/Dockerfile.new" && mv "${bundle_directory}/Dockerfile.new" "${bundle_directory}/Dockerfile"

# Include CRDs as manifests.
crd_names=$(yq eval -o=tsv '.metadata.name' ../../deploy/crd.yaml)

gawk -v names="${crd_names}" -v bundle_directory="${bundle_directory}" '
BEGIN {
    split(names, name_array, " ");
    idx=1;
}
/apiVersion: apiextensions.k8s.io\/v1/ {
    if (idx in name_array) {
        current_file = bundle_directory "/manifests/" name_array[idx] ".crd.yaml";
        idx++;
    } else {
        current_file = bundle_directory "/unnamed_" idx ".yaml";
        idx++;
    }
}
{
    if (current_file != "") {
        print > current_file;
    }
}
' ../../deploy/crd.yaml

find "${bundle_directory}/manifests" -type f -name "*.crd.yaml" -exec sed -i '' '1s/^/---\n/; ${/^---$/d;}' {} +

abort() {
	echo >&2 "$@"
	exit 1
}
dump() { yq --color-output; }

# The first command render yaml correctly and the second extract data.

yq eval -i '[.]' operator_deployments.yaml && yq eval 'length == 1' operator_deployments.yaml --exit-status >/dev/null || abort "too many deployments accounts!" $'\n'"$(yq eval . operator_deployments.yaml)"

yq eval -i '[.]' operator_accounts.yaml && yq eval 'length == 1' operator_accounts.yaml --exit-status >/dev/null || abort "too many service accounts!" $'\n'"$(yq eval . operator_accounts.yaml)"

yq eval -i '[.]' operator_roles${suffix}.yaml && yq eval 'length == 1' operator_roles${suffix}.yaml --exit-status >/dev/null || abort "too many roles!" $'\n'"$(yq eval . operator_roles${suffix}.yaml)"

# Render bundle CSV and strip comments.
export stem=$(yq -r '.projectName' "${project_directory}/PROJECT")
export version="${VERSION}${suffix}"
export timestamp=$(date -u +"%Y-%m-%dT%H:%M:%S.%3Z")
export name="${stem}.v${VERSION}${suffix}"
export name_certified="${stem}-certified.v${VERSION}${suffix}"
export name_certified_rhmp="${stem}-certified-rhmp.v${VERSION}${suffix}"
export skip_range="<v${VERSION}"
export containerImage=$(yq eval '.[0].spec.template.spec.containers[0].image' operator_deployments.yaml)
export rulesLevel=${rulesLevel}
export deployment=$(yq eval operator_deployments.yaml)
export account=$(yq eval '.[] | .metadata.name' operator_accounts.yaml)
export rules=$(yq eval '.[] | .rules' operator_roles${suffix}.yaml)
export relatedImages=$(yq eval bundle.relatedImages.yaml)

export examples=$(jq -n "[
  $(yq eval -o=json ../../deploy/cr.yaml),
  $(yq eval -o=json ../../deploy/backup/backup.yaml),
  $(yq eval -o=json ../../deploy/backup/restore.yaml)
]")

yq eval '
  .metadata.annotations["alm-examples"] = strenv(examples) |
  .metadata.annotations["containerImage"] = env(containerImage) |
  .metadata.annotations["olm.skipRange"] = env(skip_range) |
  .metadata.annotations["createdAt"] = env(timestamp) |
  .metadata.name = env(name) |
  .spec.install.spec[strenv(rulesLevel)] = [{ "serviceAccountName": env(account), "rules": env(rules) }] |
  .spec.install.spec.deployments = [( env(deployment) | .[] |{ "name": .metadata.name, "spec": .spec} )] |
  .spec.version = env(version)' bundle.csv.yaml >"${bundle_directory}/manifests/${file_name}.v${VERSION}.clusterserviceversion.yaml"

if [ "${DISTRIBUTION}" == "community" ]; then
	update_yaml_images "bundles/$DISTRIBUTION/manifests/${file_name}.v${VERSION}.clusterserviceversion.yaml"
elif [ "${DISTRIBUTION}" == "redhat" ]; then
	yq eval --inplace '
        .spec.relatedImages = env(relatedImages) |
        .metadata.annotations.certified = "true" |
        .metadata.annotations["containerImage"] = "registry.connect.redhat.com/percona/percona-xtradb-cluster-operator@sha256:<update_operator_SHA_value>" |
        .metadata.name = strenv(name_certified)' \
		"${bundle_directory}/manifests/${file_name}.v${VERSION}.clusterserviceversion.yaml"

elif [ "${DISTRIBUTION}" == "marketplace" ]; then
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
		"${bundle_directory}/manifests/${file_name}.${skip_range}.clusterserviceversion.yaml"
fi

# Delete comments
sed -i '' '/^[[:space:]]*# [^#]/d' "${bundle_directory}/manifests/${file_name}.v${VERSION}.clusterserviceversion.yaml"

# Lint the bundle YAML files.
yamllint -d '{extends: default, rules: {line-length: disable, indentation: disable}}' bundles/"$DISTRIBUTION"

if >/dev/null command -v tree; then tree -C "${bundle_directory}"; fi
