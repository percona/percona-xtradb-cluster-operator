FROM registry.access.redhat.com/ubi7/ubi-minimal
RUN microdnf update && microdnf clean all

LABEL name="Percona XtraDB Cluster Operator" \
      vendor="Percona" \
      summary="Percona XtraDB Cluster is an active/active high availability and high scalability open source solution for MySQL clustering" \
      description="Percona XtraDB Cluster is a high availability solution that helps enterprises avoid downtime and outages and meet expected customer experience." \
      maintainer="Percona Development <info@percona.com>"

COPY LICENSE /licenses/
COPY build/_output/bin/percona-xtradb-cluster-operator /usr/local/bin/percona-xtradb-cluster-operator
COPY build/pxc-entrypoint.sh /pxc-entrypoint.sh
COPY build/pxc-init-entrypoint.sh /pxc-init-entrypoint.sh
COPY build/unsafe-bootstrap.sh /unsafe-bootstrap.sh

USER nobody
