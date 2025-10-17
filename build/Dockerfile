FROM --platform=${BUILDPLATFORM} golang:1.25 AS go_builder
WORKDIR /go/src/github.com/percona/percona-xtradb-cluster-operator

COPY go.mod go.sum ./
RUN go mod download

ARG GIT_COMMIT
ARG GIT_BRANCH
ARG BUILD_TIME
ARG GO_LDFLAGS
ARG GOOS=linux
ARG TARGETARCH
ARG CGO_ENABLED=0

COPY . .

RUN mkdir -p build/_output/bin \
    && GOOS=$GOOS GOARCH=${TARGETARCH} CGO_ENABLED=$CGO_ENABLED GO_LDFLAGS=$GO_LDFLAGS \
       go build -ldflags "-w -s -X main.GitCommit=$GIT_COMMIT -X main.GitBranch=$GIT_BRANCH -X main.BuildTime=$BUILD_TIME" \
           -o build/_output/bin/percona-xtradb-cluster-operator \
               cmd/manager/main.go \
    && cp -r build/_output/bin/percona-xtradb-cluster-operator /usr/local/bin/percona-xtradb-cluster-operator

RUN GOOS=$GOOS GOARCH=${TARGETARCH} CGO_ENABLED=$CGO_ENABLED GO_LDFLAGS=$GO_LDFLAGS \
    go build -o build/_output/bin/peer-list cmd/peer-list/main.go \
    && cp -r build/_output/bin/peer-list /usr/local/bin/peer-list

RUN GOOS=$GOOS GOARCH=${TARGETARCH} CGO_ENABLED=$CGO_ENABLED GO_LDFLAGS=$GO_LDFLAGS \
       go build -o build/_output/bin/pitr \
               cmd/pitr/main.go \
    && cp -r build/_output/bin/pitr /usr/local/bin/pitr

RUN GOOS=$GOOS GOARCH=${TARGETARCH} CGO_ENABLED=$CGO_ENABLED GO_LDFLAGS=$GO_LDFLAGS \
       go build -ldflags "-w -s -X main.GitCommit=$GIT_COMMIT -X main.GitBranch=$GIT_BRANCH -X main.BuildTime=$BUILD_TIME" \
            -o build/_output/bin/mysql-state-monitor cmd/mysql-state-monitor/main.go \
    && cp -r build/_output/bin/mysql-state-monitor /usr/local/bin/mysql-state-monitor

# Looking for all possible License/Notice files and copying them to the image
RUN find $GOPATH/pkg/mod -regextype posix-extended -iregex '.*(license|notice)(\.md|\.txt|$)' \
         -exec \
            sh -c 'mkdir -pv /licenses/$(echo "$0" | sed -E "s/\/(license|notice).*$//gi") \
                   && cp -v "$0" /licenses/$(echo "$0" | sed -E "s/\/(license|notice).*$//gi")' {} \;

FROM registry.access.redhat.com/ubi9/ubi-minimal AS ubi9
RUN microdnf -y update && microdnf clean all

LABEL name="Percona XtraDB Cluster Operator" \
      vendor="Percona" \
      summary="Percona XtraDB Cluster is an active/active high availability and high scalability open source solution for MySQL clustering" \
      description="Percona XtraDB Cluster is a high availability solution that helps enterprises avoid downtime and outages and meet expected customer experience." \
      maintainer="Percona Development <info@percona.com>"

COPY LICENSE /licenses/
COPY --from=go_builder /usr/local/bin/percona-xtradb-cluster-operator /usr/local/bin/percona-xtradb-cluster-operator
COPY --from=go_builder /usr/local/bin/peer-list /peer-list
COPY --from=go_builder /usr/local/bin/pitr /pitr
COPY --from=go_builder /usr/local/bin/mysql-state-monitor /mysql-state-monitor
COPY build/pxc-entrypoint.sh /pxc-entrypoint.sh
COPY build/pxc-init-entrypoint.sh /pxc-init-entrypoint.sh
COPY build/pitr-init-entrypoint.sh /pitr-init-entrypoint.sh
COPY build/backup-init-entrypoint.sh /backup-init-entrypoint.sh
COPY build/pxc-configure-pxc.sh /pxc-configure-pxc.sh
COPY build/liveness-check.sh /liveness-check.sh
COPY build/readiness-check.sh /readiness-check.sh
COPY build/pmm-prerun.sh /pmm-prerun.sh
COPY build/get-pxc-state /get-pxc-state
COPY build/wsrep_cmd_notify_handler.sh /wsrep_cmd_notify_handler.sh
COPY build/prepare_restored_cluster.sh /prepare_restored_cluster.sh

COPY build/haproxy-entrypoint.sh /haproxy-entrypoint.sh
COPY build/haproxy-init-entrypoint.sh /haproxy-init-entrypoint.sh
COPY build/haproxy_add_pxc_nodes.sh /haproxy_add_pxc_nodes.sh
COPY build/haproxy_check_pxc.sh /haproxy_check_pxc.sh
COPY build/haproxy_liveness_check.sh /haproxy_liveness_check.sh
COPY build/haproxy_readiness_check.sh /haproxy_readiness_check.sh
COPY build/haproxy.cfg /haproxy.cfg
COPY build/haproxy-global.cfg /haproxy-global.cfg

COPY build/proxysql-entrypoint.sh /proxysql-entrypoint.sh
COPY build/proxysql-init-entrypoint.sh /proxysql-init-entrypoint.sh
COPY build/proxysql.cnf /proxysql.cnf
COPY build/proxysql-admin.cnf /proxysql-admin.cnf
COPY build/proxysql_add_cluster_nodes.sh /proxysql_add_cluster_nodes.sh
COPY build/proxysql_add_proxysql_nodes.sh /proxysql_add_proxysql_nodes.sh
COPY build/proxysql_add_pxc_nodes.sh /proxysql_add_pxc_nodes.sh

COPY build/backup /backup

USER 2
