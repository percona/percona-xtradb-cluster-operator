FROM golang:1.16 AS go_builder
WORKDIR /go/src/github.com/percona/percona-xtradb-cluster-operator

COPY . .

ARG GIT_COMMIT
ARG GIT_BRANCH
ARG BUILD_TIME
ARG GO_LDFLAGS
ARG GOOS=linux
ARG GOARCH=amd64
ARG CGO_ENABLED=0

RUN go get k8s.io/apimachinery/pkg/util/sets \
    && go build -o build/_output/bin/peer-list cmd/peer-list/main.go \
    && cp -r build/_output/bin/peer-list /usr/local/bin/

FROM registry.access.redhat.com/ubi7/ubi-minimal AS ubi7
RUN microdnf update && microdnf clean all

LABEL name="Percona XtraDB Cluster Operator" \
      vendor="Percona" \
      summary="Percona XtraDB Cluster is an active/active high availability and high scalability open source solution for MySQL clustering" \
      description="Percona XtraDB Cluster is a high availability solution that helps enterprises avoid downtime and outages and meet expected customer experience." \
      maintainer="Percona Development <info@percona.com>"

COPY LICENSE /licenses/
COPY --from=go_builder /usr/local/bin/peer-list /peer-list
COPY build/ps-entrypoint.sh /ps-entrypoint.sh
COPY build/ps-init-entrypoint.sh /ps-init-entrypoint.sh

USER nobody
