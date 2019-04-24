FROM alpine:3.9
RUN apk upgrade --update --no-cache && rm -rf /var/cache/apk/*

MAINTAINER Percona Development <info@percona.com>
LABEL name="Percona XtraDB Cluster Operator" \
      vendor="Percona" \
      summary="Percona XtraDB Cluster is an active/active high availability and high scalability open source solution for MySQL clustering" \
      description="Percona XtraDB Cluster is a high availability solution that helps enterprises avoid downtime and outages and meet expected customer experience."

COPY LICENSE /licenses/
COPY build/_output/bin/percona-xtradb-cluster-operator /usr/local/bin/percona-xtradb-cluster-operator

USER nobody
