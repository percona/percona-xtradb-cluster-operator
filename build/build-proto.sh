#!/bin/sh

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

if ! protoc --version &>/dev/null; then
  apt-get update && apt-get install unzip

  curl -LO https://github.com/protocolbuffers/protobuf/releases/download/v3.12.1/protoc-3.12.1-linux-x86_64.zip

  unzip protoc-3.12.1-linux-x86_64.zip -d $HOME/.local

  proto_compiler=$HOME/.local/bin/protoc
fi

rm -rf build/third_party

mkdir -p openapi
mkdir -p build/third_party/openapi_proto

git clone https://github.com/googleapis/googleapis build/third_party/googleapis
git clone -b v2 https://github.com/grpc-ecosystem/grpc-gateway build/third_party/openapi

cd build/third_party/googleapis && git checkout 76905ffe7e3b0605f64ef889fb88913634f9f836 && cd ../../..
cd build/third_party/openapi && git checkout fcec199c8378122670cbf93a72d7363e9bd460fe && mv protoc-gen-openapiv2 ../openapi_proto/ && cd ../../..

$proto_compiler -I build/third_party/googleapis -I build/third_party/openapi_proto -I vendor/github.com/Percona-Lab/percona-version-service/api --openapiv2_out=./openapi vendor/github.com/Percona-Lab/percona-version-service/api/version.proto

swagger generate client -f openapi/version.swagger.json -c versionserviceclient -m versionserviceclient/models

rm -rf openapi
rm -rf build/third_party
