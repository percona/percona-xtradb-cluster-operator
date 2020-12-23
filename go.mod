module github.com/percona/percona-xtradb-cluster-operator

go 1.13

require (
	github.com/Percona-Lab/percona-version-service/api v0.0.0-20200714141734-e9fed619b55c
	github.com/caarlos0/env v3.5.0+incompatible // indirect
	github.com/go-ini/ini v1.25.4
	github.com/go-openapi/errors v0.19.6
	github.com/go-openapi/runtime v0.19.16
	github.com/go-openapi/strfmt v0.19.5
	github.com/go-openapi/swag v0.19.9
	github.com/go-openapi/validate v0.19.10
	github.com/go-sql-driver/mysql v1.4.1
	github.com/google/go-cmp v0.4.1 // indirect
	github.com/hashicorp/go-version v1.1.0
	github.com/jetstack/cert-manager v0.15.1
	github.com/minio/minio-go v6.0.14+incompatible
	github.com/minio/minio-go/v7 v7.0.6
	github.com/operator-framework/operator-sdk v0.17.1
	github.com/pierrec/lz4 v2.6.0+incompatible
	github.com/pkg/errors v0.9.1
	github.com/robfig/cron/v3 v3.0.1
	golang.org/x/mod v0.3.0 // indirect
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d // indirect
	golang.org/x/tools v0.0.0-20200612220849-54c614fe050c // indirect
	google.golang.org/appengine v1.6.6 // indirect
	google.golang.org/protobuf v1.24.0 // indirect
	k8s.io/api v0.18.6
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kubernetes v1.13.0
	sigs.k8s.io/controller-runtime v0.6.2

)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	k8s.io/api => k8s.io/api v0.18.6 // Required by client-go
	k8s.io/apimachinery => k8s.io/apimachinery v0.18.6 // Required by client-go
	k8s.io/client-go => k8s.io/client-go v0.18.6 // Required by prometheus-operator
)
