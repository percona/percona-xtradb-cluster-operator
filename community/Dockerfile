# Used to build the bundle image. This file is ignored by the community operator
# registries which work with bundle directories instead.
# https://operator-framework.github.io/community-operators/packaging-operator/

FROM scratch AS builder

COPY manifests/ /build/manifests/
COPY metadata/ /build/metadata/

FROM scratch

# LABELS is replaced with bundle.annotations.yaml
LABEL operators.operatorframework.io.bundle.mediatype.v1="registry+v1"

LABEL operators.operatorframework.io.bundle.manifests.v1="manifests/"

LABEL operators.operatorframework.io.bundle.metadata.v1="metadata/"

LABEL operators.operatorframework.io.bundle.package.v1="percona-xtradb-cluster-operator"

LABEL operators.operatorframework.io.bundle.channels.v1="stable"

LABEL operators.operatorframework.io.bundle.channel.default.v1="stable"

LABEL com.redhat.openshift.versions="v4.14-v4.18"

LABEL org.opencontainers.image.authors="info@percona.com"

LABEL org.opencontainers.image.url="https://percona.com"

LABEL org.opencontainers.image.vendor="Percona"
LABEL com.redhat.delivery.backport=true
LABEL com.redhat.delivery.operator.bundle=true

COPY --from=builder /build/ /
