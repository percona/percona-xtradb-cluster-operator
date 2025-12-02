package api

// Run `make protoc` to install protoc and protoc-gen-go
//go:generate ../../../bin/protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative app.proto
