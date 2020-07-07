#!/bin/sh

set -o errexit
set -o xtrace

if ! swagger version &>/dev/null; then
  go install github.com/go-swagger/go-swagger/cmd/swagger
fi

rm -rf versionserviceclient

swagger generate client -f vendor/github.com/Percona-Lab/percona-version-service/api/version.swagger.yaml -c versionserviceclient -m versionserviceclient/models
