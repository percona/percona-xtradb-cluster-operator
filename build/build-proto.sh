#!/bin/bash

set -o errexit
set -o xtrace

go get \
	github.com/golang/protobuf/protoc-gen-go \
	github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway \
	github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2 \
	github.com/rakyll/statik

go install github.com/go-swagger/go-swagger/cmd/swagger

protoc -I build/third_party/googleapis -I build/third_party/grpc-gateway -I vendor/github.com/Percona-Lab/percona-version-service/api --openapiv2_out=./openapi vendor/github.com/Percona-Lab/percona-version-service/api/version.proto

swagger generate client -f openapi/version.swagger.json -c versionserviceclient -m versionserviceclient/models
