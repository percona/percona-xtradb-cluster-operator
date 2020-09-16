module github.com/percona/percona-xtradb-cluster-operator

go 1.13

require (
	github.com/Percona-Lab/percona-version-service/api v0.0.0-20200915125717-b7022d6bfca1
	github.com/go-ini/ini v1.51.1
	github.com/go-openapi/errors v0.19.2
	github.com/go-openapi/runtime v0.19.4
	github.com/go-openapi/strfmt v0.19.3
	github.com/go-openapi/swag v0.19.5
	github.com/go-openapi/validate v0.19.5
	github.com/go-sql-driver/mysql v1.5.0
	github.com/hashicorp/go-version v1.2.0
	github.com/jetstack/cert-manager v0.15.1
	github.com/operator-framework/operator-sdk v0.17.1
	github.com/pkg/errors v0.9.1
	github.com/robfig/cron/v3 v3.0.1
	github.com/stretchr/testify v1.5.1 // indirect
	go.mongodb.org/mongo-driver v1.3.4 // indirect
	k8s.io/api v0.18.0
	k8s.io/apimachinery v0.18.0
	k8s.io/client-go v12.0.0+incompatible
	sigs.k8s.io/controller-runtime v0.5.2
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	k8s.io/api => k8s.io/api v0.17.4 // Required by client-go
	k8s.io/apimachinery => k8s.io/apimachinery v0.17.4 // Required by client-go
	k8s.io/client-go => k8s.io/client-go v0.17.4 // Required by prometheus-operator
)
