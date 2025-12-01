NAME ?= percona-xtradb-cluster-operator
IMAGE_TAG_OWNER ?= perconalab
IMAGE_TAG_BASE ?= $(IMAGE_TAG_OWNER)/$(NAME)
SED := $(shell which gsed || which sed)
VERSION ?= $(shell git rev-parse --abbrev-ref HEAD | $(SED) -e 's^/^-^g; s^[.]^-^g;' | tr '[:upper:]' '[:lower:]')
IMAGE ?= $(IMAGE_TAG_BASE):$(VERSION)
DEPLOYDIR = ./deploy
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
# Using a fixed version instead of 'latest' because on GitHub Actions,
# the 'latest' tag sometimes resolves to an older or incompatible version,
# leading to test or pipeline failures.
ENVTEST_K8S_VERSION = 1.34.1

all: build

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

generate: controller-gen  ## Generate CRDs and RBAC files
	go generate ./...
	$(CONTROLLER_GEN) crd:maxDescLen=0,allowDangerousTypes=true,generateEmbeddedObjectMeta=true rbac:roleName=$(NAME) webhook paths="./..." output:crd:artifacts:config=config/crd/bases  ## Generate WebhookConfiguration, Role and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) object paths="./..." ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.

$(DEPLOYDIR)/crd.yaml: kustomize generate
	$(KUSTOMIZE) build config/crd/ > $(DEPLOYDIR)/crd.yaml

.PHONY: $(DEPLOYDIR)/operator.yaml
$(DEPLOYDIR)/operator.yaml:
	$(SED) -i "/^      containers:/,/^        image:/{s#image: .*#image: $(IMAGE_TAG_BASE):$(VERSION)#}" deploy/operator.yaml

.PHONY: $(DEPLOYDIR)/cw-operator.yaml
$(DEPLOYDIR)/cw-operator.yaml:
	$(SED) -i "/^      containers:/,/^        image:/{s#image: .*#image: $(IMAGE_TAG_BASE):$(VERSION)#}" deploy/cw-operator.yaml

$(DEPLOYDIR)/bundle.yaml: $(DEPLOYDIR)/crd.yaml $(DEPLOYDIR)/rbac.yaml $(DEPLOYDIR)/operator.yaml  ## Generate deploy/bundle.yaml
	cat $(DEPLOYDIR)/crd.yaml > $(DEPLOYDIR)/bundle.yaml; echo "---" >> $(DEPLOYDIR)/bundle.yaml; cat $(DEPLOYDIR)/rbac.yaml >> $(DEPLOYDIR)/bundle.yaml; echo "---" >> $(DEPLOYDIR)/bundle.yaml; cat $(DEPLOYDIR)/operator.yaml >> $(DEPLOYDIR)/bundle.yaml

$(DEPLOYDIR)/cw-bundle.yaml: $(DEPLOYDIR)/crd.yaml $(DEPLOYDIR)/cw-rbac.yaml $(DEPLOYDIR)/cw-operator.yaml  ## Generate deploy/cw-bundle.yaml
	cat $(DEPLOYDIR)/crd.yaml > $(DEPLOYDIR)/cw-bundle.yaml; echo "---" >> $(DEPLOYDIR)/cw-bundle.yaml; cat $(DEPLOYDIR)/cw-rbac.yaml >> $(DEPLOYDIR)/cw-bundle.yaml; echo "---" >> $(DEPLOYDIR)/cw-bundle.yaml; cat $(DEPLOYDIR)/cw-operator.yaml >> $(DEPLOYDIR)/cw-bundle.yaml

manifests: $(DEPLOYDIR)/crd.yaml $(DEPLOYDIR)/bundle.yaml $(DEPLOYDIR)/cw-bundle.yaml ## Put generated manifests to deploy directory

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

test: manifests generate fmt vet envtest ## Run tests.
	DISABLE_TELEMETRY=true KUBEBUILDER_ASSETS="$(shell $(ENVTEST) --arch=amd64 use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out

e2e-test:
	IMAGE=$(IMAGE) ./e2e-tests/$(TEST)/run

##@ Build

.PHONY: build
build: generate ## Build docker image for operator
	VERSION=$(VERSION) IMAGE=$(IMAGE) ./e2e-tests/build

##@ Deployment

install: manifests ## Install CRDs, rbac
	kubectl apply --server-side -f $(DEPLOYDIR)/crd.yaml
	kubectl apply -f $(DEPLOYDIR)/rbac.yaml

uninstall: manifests ## Uninstall CRDs, rbac
	kubectl delete -f $(DEPLOYDIR)/crd.yaml
	kubectl delete -f $(DEPLOYDIR)/rbac.yaml

.PHONY: deploy
deploy: ## Deploy operator
	yq eval '(.spec.template.spec.containers[] | select(.name=="$(NAME)") | .image) = "$(IMAGE)"' $(DEPLOYDIR)/operator.yaml \
		| yq eval '(.spec.template.spec.containers[] | select(.name=="$(NAME)") | .env[] | select(.name=="DISABLE_TELEMETRY") | .value) = "true"' - \
		| yq eval '(.spec.template.spec.containers[] | select(.name=="$(NAME)") | .env[] | select(.name=="LOG_LEVEL") | .value) = "DEBUG"' - \
		| kubectl apply -f -

undeploy: ## Undeploy operator
	kubectl delete -f $(DEPLOYDIR)/operator.yaml

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.19.0)

KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@v4.5.3)

ENVTEST = $(shell pwd)/bin/setup-envtest
envtest: ## Download envtest-setup locally if necessary.
	$(call go-get-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

SWAGGER = $(shell pwd)/bin/swagger
swagger: ## Download swagger locally if necessary.
	$(call go-get-tool,$(SWAGGER),github.com/go-swagger/go-swagger/cmd/swagger@latest)

PROTOC_VERSION = 33.1
PROTOC = $(shell pwd)/bin/protoc
protoc: ## Download protoc locally if necessary.
	os='linux'; \
	arch='x86_64'; \
	if [ "$(shell uname)" = "Darwin" ]; then \
		os='osx'; \
	fi; \
	if [ "$(shell uname -m)" = "arm64" ]; then \
		arch='aarch_64'; \
	fi; \
	curl -LO "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-$${os}-$${arch}.zip"; \
	unzip -o protoc-${PROTOC_VERSION}-$${os}-$${arch}.zip -d protoc-${PROTOC_VERSION}-$${os}-$${arch}; \
	rm protoc-${PROTOC_VERSION}-$${os}-$${arch}.zip; \
	mv -f protoc-${PROTOC_VERSION}-$${os}-$${arch}/bin/protoc $(PROTOC); \
	rm -rf protoc-${PROTOC_VERSION}-$${os}-$${arch}; \
	$(call go install google.golang.org/protobuf/cmd/protoc-gen-go@latest)

# Prepare release
include e2e-tests/release_versions
CERT_MANAGER_VER := $(shell grep -Eo "cert-manager v.*" go.mod|grep -Eo "[0-9]+\.[0-9]+\.[0-9]+")
release: manifests
	$(SED) -i "/CERT_MANAGER_VER/s/CERT_MANAGER_VER=\".*/CERT_MANAGER_VER=\"$(CERT_MANAGER_VER)\"/" e2e-tests/functions
	echo $(VERSION) > pkg/version/version.txt
	$(SED) -i \
		-e "s/crVersion: .*/crVersion: $(VERSION)/" \
		-e "/^  pxc:/,/^    image:/{s#image: .*#image: $(IMAGE_PXC80)#}" \
		-e "/^  haproxy:/,/^    image:/{s#image: .*#image: $(IMAGE_HAPROXY)#}" \
		-e "/^  logcollector:/,/^    image:/{s#image: .*#image: $(IMAGE_LOGCOLLECTOR)#}" deploy/cr-minimal.yaml
	$(SED) -i \
		-e "s/crVersion: .*/crVersion: $(VERSION)/" \
		-e "/^  pxc:/,/^    image:/{s#image: .*#image: $(IMAGE_PXC80)#}" \
		-e "/^  haproxy:/,/^    image:/{s#image: .*#image: $(IMAGE_HAPROXY)#}" \
		-e "/^  proxysql:/,/^    image:/{s#image: .*#image: $(IMAGE_PROXY)#}" \
		-e "/^  logcollector:/,/^    image:/{s#image: .*#image: $(IMAGE_LOGCOLLECTOR)#}" \
		-e "/^  backup:/,/^    image:/{s#image: .*#image: $(IMAGE_BACKUP80)#}" \
		-e "/initContainer:/,/image:/{s#image: .*#image: $(IMAGE_OPERATOR)#}" \
		-e "/^  pmm:/,/^    image:/{s#image: .*#image: $(IMAGE_PMM_CLIENT)#}" deploy/cr.yaml

# Prepare main branch after release
MAJOR_VER := $(shell grep -oE "crVersion: .*" deploy/cr.yaml|grep -oE "[0-9]+\.[0-9]+\.[0-9]+"|cut -d'.' -f1)
MINOR_VER := $(shell grep -oE "crVersion: .*" deploy/cr.yaml|grep -oE "[0-9]+\.[0-9]+\.[0-9]+"|cut -d'.' -f2)
NEXT_VER ?= $(MAJOR_VER).$$(($(MINOR_VER) + 1)).0
after-release: manifests
	echo $(NEXT_VER) > pkg/version/version.txt
	$(SED) -i \
		-e "s/crVersion: .*/crVersion: $(NEXT_VER)/" \
		-e "/^  pxc:/,/^    image:/{s#image: .*#image: perconalab/percona-xtradb-cluster-operator:main-pxc8.0#}" \
		-e "/^  haproxy:/,/^    image:/{s#image: .*#image: perconalab/percona-xtradb-cluster-operator:main-haproxy#}" \
		-e "/^  logcollector:/,/^    image:/{s#image: .*#image: perconalab/percona-xtradb-cluster-operator:main-logcollector#}" deploy/cr-minimal.yaml
	$(SED) -i \
		-e "s/crVersion: .*/crVersion: $(NEXT_VER)/" \
		-e "/^  pxc:/,/^    image:/{s#image: .*#image: perconalab/percona-xtradb-cluster-operator:main-pxc8.0#}" \
		-e "/^  haproxy:/,/^    image:/{s#image: .*#image: perconalab/percona-xtradb-cluster-operator:main-haproxy#}" \
		-e "/^  proxysql:/,/^    image:/{s#image: .*#image: perconalab/percona-xtradb-cluster-operator:main-proxysql#}" \
		-e "/^  logcollector:/,/^    image:/{s#image: .*#image: perconalab/percona-xtradb-cluster-operator:main-logcollector#}" \
		-e "/^  backup:/,/^    image:/{s#image: .*#image: perconalab/percona-xtradb-cluster-operator:main-pxc8.0-backup#}" \
		-e "/initContainer:/,/image:/{s#image: .*#image: perconalab/percona-xtradb-cluster-operator:main#}" \
		-e "/^  pmm:/,/^    image:/{s#image: .*#image: perconalab/pmm-client:dev-latest#}" deploy/cr.yaml

VS_BRANCH = main
version-service-client: swagger
	curl https://raw.githubusercontent.com/Percona-Lab/percona-version-service/$(VS_BRANCH)/api/version.swagger.yaml \
		--output ./version.swagger.yaml
	rm -rf ./pkg/version/client
	mkdir -p ./pkg/version/client/models
	mkdir -p ./pkg/version/client/version_service
	./bin/swagger generate client \
		-f ./pkg/version.swagger.yaml \
		-c ./pkg/version/client \
		-m ./pkg/version/client/models
	rm ./version.swagger.yaml
