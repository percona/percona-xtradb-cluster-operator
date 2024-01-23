NAME ?= percona-xtradb-cluster-operator
IMAGE_TAG_OWNER ?= perconalab
IMAGE_TAG_BASE ?= $(IMAGE_TAG_OWNER)/$(NAME)
VERSION ?= $(shell git rev-parse --abbrev-ref HEAD | sed -e 's^/^-^g; s^[.]^-^g;' | tr '[:upper:]' '[:lower:]')
IMAGE ?= $(IMAGE_TAG_BASE):$(VERSION)
DEPLOYDIR = ./deploy

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.23

all: build

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

generate: controller-gen  ## Generate CRDs and RBAC files
	$(CONTROLLER_GEN) crd:maxDescLen=0,allowDangerousTypes=true rbac:roleName=$(NAME) webhook paths="./..." output:crd:artifacts:config=config/crd/bases  ## Generate WebhookConfiguration, Role and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) object paths="./..." ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.

$(DEPLOYDIR)/crd.yaml: kustomize generate
	$(KUSTOMIZE) build config/crd/ > $(DEPLOYDIR)/crd.yaml

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
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.8.0)

KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@v4.5.3)

ENVTEST = $(shell pwd)/bin/setup-envtest
envtest: ## Download envtest-setup locally if necessary.
	$(call go-get-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

