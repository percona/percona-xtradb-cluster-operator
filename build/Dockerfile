FROM alpine:3.8

RUN apk upgrade --update --no-cache

USER nobody

ADD build/_output/bin/percona-xtradb-cluster-operator /usr/local/bin/percona-xtradb-cluster-operator
