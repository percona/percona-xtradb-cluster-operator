package api

// Run `make protoc` to install protoc and protoc-gen-go
//go:generate ../../../bin/protoc --plugin=protoc-gen-go=../../../bin/protoc-gen-go --plugin=protoc-gen-go-grpc=../../../bin/protoc-gen-go-grpc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative app.proto
