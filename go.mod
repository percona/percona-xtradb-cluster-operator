module github.com/percona/percona-xtradb-cluster-operator

go 1.13

require (
	github.com/Percona-Lab/percona-version-service/api v0.0.0-20201216104127-a39f2dded3cc
	github.com/caarlos0/env v3.5.0+incompatible
	github.com/go-ini/ini v1.25.4
	github.com/go-logr/logr v0.3.0
	github.com/go-logr/zapr v0.2.0
	github.com/go-openapi/errors v0.19.8
	github.com/go-openapi/runtime v0.19.20
	github.com/go-openapi/strfmt v0.20.3
	github.com/go-openapi/swag v0.19.9
	github.com/go-openapi/validate v0.19.10
	github.com/go-sql-driver/mysql v1.5.0
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-cmp v0.5.5
	github.com/google/uuid v1.1.2 // indirect
	github.com/hashicorp/go-version v1.1.0
	github.com/jetstack/cert-manager v1.0.4
	github.com/minio/minio-go/v7 v7.0.6
	github.com/operator-framework/operator-sdk v0.19.4
	github.com/pkg/errors v0.9.1
	github.com/robfig/cron/v3 v3.0.1
	go.uber.org/atomic v1.8.0 // indirect
	go.uber.org/multierr v1.7.0 // indirect
	go.uber.org/zap v1.18.1
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519 // indirect
	golang.org/x/mod v0.4.0 // indirect
	golang.org/x/oauth2 v0.0.0-20200902213428-5d25da1a8d43 // indirect
	golang.org/x/tools v0.0.0-20201211185031-d93e913c1a58 // indirect
	google.golang.org/protobuf v1.27.1 // indirect
	k8s.io/api v0.19.2
	k8s.io/apimachinery v0.19.2
	k8s.io/client-go v12.0.0+incompatible
	sigs.k8s.io/controller-runtime v0.7.0

)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	k8s.io/api => k8s.io/api v0.19.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.19.2
	k8s.io/client-go => k8s.io/client-go v0.19.2 // Required by prometheus-operator
)
