package xtrabackupserversidecar

import (
	"fmt"
	"log"
	"net"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup/api"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup/server"
	"google.golang.org/grpc"
)

func main() {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", server.DefaultPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	var serverOptions []grpc.ServerOption
	grpcServ := grpc.NewServer(serverOptions...)
	api.RegisterXtrabackupServiceServer(grpcServ, server.New())

	log.Printf("server listening at %v", lis.Addr())
	if err := grpcServ.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
