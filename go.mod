module github.com/percona/percona-xtradb-cluster-operator

go 1.18

require (
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v0.5.1
	github.com/Percona-Lab/percona-version-service/api v0.0.0-20201216104127-a39f2dded3cc
	github.com/caarlos0/env v3.5.0+incompatible
	github.com/cert-manager/cert-manager v1.10.0
	github.com/flosch/pongo2/v6 v6.0.0
	github.com/go-ini/ini v1.67.0
	github.com/go-logr/logr v1.2.3
	github.com/go-logr/zapr v1.2.3
	github.com/go-openapi/errors v0.20.3
	github.com/go-openapi/runtime v0.24.2
	github.com/go-openapi/strfmt v0.21.3
	github.com/go-openapi/swag v0.22.3
	github.com/go-openapi/validate v0.22.0
	github.com/go-sql-driver/mysql v1.6.0
	github.com/google/go-cmp v0.5.9
	github.com/hashicorp/go-version v1.6.0
	github.com/minio/minio-go/v7 v7.0.43
	github.com/pkg/errors v0.9.1
	github.com/robfig/cron/v3 v3.0.1
	go.uber.org/zap v1.23.0
	golang.org/x/sync v0.1.0
	k8s.io/api v0.25.3
	k8s.io/apimachinery v0.25.3
	k8s.io/client-go v0.25.3
	k8s.io/klog/v2 v2.80.1
	sigs.k8s.io/controller-runtime v0.13.0
)

require (
	cloud.google.com/go v0.99.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.1.4 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.0.1 // indirect
	github.com/asaskevich/govalidator v0.0.0-20210307081110-f21760c49a8d // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/emicklei/go-restful/v3 v3.8.0 // indirect
	github.com/evanphx/json-patch v5.6.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.6.0 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/go-openapi/analysis v0.21.4 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.20.0 // indirect
	github.com/go-openapi/loads v0.21.2 // indirect
	github.com/go-openapi/spec v0.20.7 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/gnostic v0.6.9 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/klauspost/cpuid/v2 v2.1.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/minio/sha256-simd v1.0.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/prometheus/client_golang v1.13.0 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/rs/xid v1.4.0 // indirect
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.mongodb.org/mongo-driver v1.10.3 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.7.0 // indirect
	golang.org/x/crypto v0.0.0-20220924013350-4ba4fb4dd9e7 // indirect
	golang.org/x/net v0.0.0-20220921155015-db77216a4ee9 // indirect
	golang.org/x/oauth2 v0.0.0-20220909003341-f21342109be1 // indirect
	golang.org/x/sys v0.1.0 // indirect
	golang.org/x/term v0.1.0 // indirect
	golang.org/x/text v0.3.8 // indirect
	golang.org/x/time v0.0.0-20220609170525-579cf78fd858 // indirect
	gomodules.xyz/jsonpatch/v2 v2.2.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.66.6 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiextensions-apiserver v0.25.2 // indirect
	k8s.io/component-base v0.25.2 // indirect
	k8s.io/kube-openapi v0.0.0-20220803164354-a70c9af30aea // indirect
	k8s.io/utils v0.0.0-20220922133306-665eaaec4324 // indirect
	sigs.k8s.io/gateway-api v0.5.0 // indirect
	sigs.k8s.io/json v0.0.0-20220713155537-f223a00ba0e2 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM

exclude (
	github.com/gogo/protobuf v1.1.1
	github.com/gogo/protobuf v1.2.1
	github.com/gogo/protobuf v1.3.1
	go.mongodb.org/mongo-driver v1.0.3
	go.mongodb.org/mongo-driver v1.0.4
	go.mongodb.org/mongo-driver v1.1.0
	go.mongodb.org/mongo-driver v1.1.1
	go.mongodb.org/mongo-driver v1.1.2
	go.mongodb.org/mongo-driver v1.1.3
	go.mongodb.org/mongo-driver v1.1.4
	go.mongodb.org/mongo-driver v1.2.0
	go.mongodb.org/mongo-driver v1.2.1
	go.mongodb.org/mongo-driver v1.3.0
	go.mongodb.org/mongo-driver v1.3.1
	go.mongodb.org/mongo-driver v1.3.2
	go.mongodb.org/mongo-driver v1.3.3
	go.mongodb.org/mongo-driver v1.3.4
	go.mongodb.org/mongo-driver v1.3.5
	go.mongodb.org/mongo-driver v1.3.6
	go.mongodb.org/mongo-driver v1.3.7
	go.mongodb.org/mongo-driver v1.4.0
	go.mongodb.org/mongo-driver v1.4.0-beta1
	go.mongodb.org/mongo-driver v1.4.0-beta2
	go.mongodb.org/mongo-driver v1.4.0-rc0
	go.mongodb.org/mongo-driver v1.4.1
	go.mongodb.org/mongo-driver v1.4.2
	go.mongodb.org/mongo-driver v1.4.3
	go.mongodb.org/mongo-driver v1.4.4
	go.mongodb.org/mongo-driver v1.4.5
	go.mongodb.org/mongo-driver v1.4.6
	go.mongodb.org/mongo-driver v1.4.7
	go.mongodb.org/mongo-driver v1.5.0
	go.mongodb.org/mongo-driver v1.5.0-beta1
)
