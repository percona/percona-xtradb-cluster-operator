module github.com/percona/percona-xtradb-cluster-operator

go 1.13

require (
	github.com/Percona-Lab/percona-version-service/api v0.0.0-20200707073643-e3dd469db8be
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/go-openapi/errors v0.19.6
	github.com/go-openapi/runtime v0.19.16
	github.com/go-openapi/strfmt v0.19.5
	github.com/go-openapi/swag v0.19.9
	github.com/go-openapi/validate v0.19.10
	github.com/go-sql-driver/mysql v1.4.1
	github.com/golang/protobuf v1.4.2 // indirect
	github.com/google/go-cmp v0.4.1 // indirect
	github.com/hashicorp/go-version v1.1.0
	github.com/jetstack/cert-manager v0.15.1
	github.com/operator-framework/operator-sdk v0.17.1
	github.com/pkg/errors v0.9.1
	github.com/robfig/cron/v3 v3.0.1
	golang.org/x/crypto v0.0.0-20200604202706-70a84ac30bf9 // indirect
	golang.org/x/mod v0.3.0 // indirect
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d // indirect
	golang.org/x/sys v0.0.0-20200610111108-226ff32320da // indirect
	golang.org/x/tools v0.0.0-20200612220849-54c614fe050c // indirect
	google.golang.org/appengine v1.6.6 // indirect
	google.golang.org/protobuf v1.24.0 // indirect
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
