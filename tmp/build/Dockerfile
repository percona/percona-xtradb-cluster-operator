FROM alpine:3.6

RUN adduser -D percona-xtradb-cluster-operator
USER percona-xtradb-cluster-operator

ADD tmp/_output/bin/percona-xtradb-cluster-operator /usr/local/bin/percona-xtradb-cluster-operator
