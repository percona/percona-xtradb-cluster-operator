FROM --platform=${BUILDPLATFORM} golang:1.26 AS go_builder
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
       go build -o build/_output/bin/xtrabackup-server-sidecar \
               cmd/xtrabackup/server/main.go \
    && cp -r build/_output/bin/xtrabackup-server-sidecar /usr/local/bin/xtrabackup-server-sidecar
    
RUN GOOS=$GOOS GOARCH=${TARGETARCH} CGO_ENABLED=$CGO_ENABLED GO_LDFLAGS=$GO_LDFLAGS \
       go build -o build/_output/bin/xtrabackup-run-backup \
               cmd/xtrabackup/run-backup/main.go \
    && cp -r build/_output/bin/xtrabackup-run-backup /usr/local/bin/xtrabackup-run-backup

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
RUN microdnf -y update && microdnf reinstall tzdata -y && microdnf clean all

LABEL name="Percona XtraDB Cluster Operator" \
      vendor="Percona" \
      summary="Percona XtraDB Cluster is an active/active high availability and high scalability open source solution for MySQL clustering" \
      description="Percona XtraDB Cluster is a high availability solution that helps enterprises avoid downtime and outages and meet expected customer experience." \
      maintainer="Percona Development <info@percona.com>"

COPY LICENSE /licenses/
COPY --from=go_builder /usr/local/bin/percona-xtradb-cluster-operator /usr/local/bin/percona-xtradb-cluster-operator
COPY --from=go_builder \
     /usr/local/bin/peer-list \
     /usr/local/bin/pitr \
     /usr/local/bin/mysql-state-monitor \
     /usr/local/bin/xtrabackup-server-sidecar \
     /usr/local/bin/xtrabackup-run-backup \
     /


COPY build/pxc-entrypoint.sh \
     build/pxc-init-entrypoint.sh \
     build/pitr-init-entrypoint.sh \
     build/backup-init-entrypoint.sh \
     build/pxc-configure-pxc.sh \
     build/liveness-check.sh \
     build/readiness-check.sh \
     build/pmm-prerun.sh \
     build/get-pxc-state \
     build/wsrep_cmd_notify_handler.sh \
     build/prepare_restored_cluster.sh \
     /

COPY build/haproxy-entrypoint.sh \
     build/haproxy-init-entrypoint.sh \
     build/haproxy_add_pxc_nodes.sh \
     build/haproxy_check_pxc.sh \
     build/haproxy_liveness_check.sh \
     build/haproxy_readiness_check.sh \
     build/haproxy.cfg \
     build/haproxy-global.cfg \
     /

COPY build/proxysql-entrypoint.sh \
     build/proxysql-init-entrypoint.sh \
     build/proxysql.cnf \
     build/proxysql-admin.cnf \
     build/proxysql_add_cluster_nodes.sh \
     build/proxysql_add_proxysql_nodes.sh \
     build/proxysql_add_pxc_nodes.sh \
     build/proxysql_scheduler_config.tmpl \
     build/proxysql_readiness_check.sh \
     build/proxysql_liveness_check.sh \
     /

COPY build/logcollector /logcollector

COPY build/backup /backup

USER 2
