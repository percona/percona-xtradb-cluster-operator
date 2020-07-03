#!/bin/bash

set -o errexit
set -o xtrace

proto_compiler=protoc

go get \
	github.com/golang/protobuf/protoc-gen-go \
	github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway \
	github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2 \
	github.com/rakyll/statik

go install github.com/go-swagger/go-swagger/cmd/swagger

rm -rf versionserviceclient

if ! protoc --version &> /dev/null
then
  apt-get update && apt-get install unzip

  curl -LO https://github.com/protocolbuffers/protobuf/releases/download/v3.12.1/protoc-3.12.1-linux-x86_64.zip

  unzip protoc-3.12.1-linux-x86_64.zip -d $HOME/.local

  proto_compiler=$HOME/.local/bin/protoc

fi

mkdir -p openapi

$proto_compiler -I build/third_party/googleapis -I build/third_party/grpc-gateway -I vendor/github.com/Percona-Lab/percona-version-service/api --openapiv2_out=./openapi vendor/github.com/Percona-Lab/percona-version-service/api/version.proto

swagger generate client -f openapi/version.swagger.json -c versionserviceclient -m versionserviceclient/models
