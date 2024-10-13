NAME ?= percona-xtradb-cluster-operator
IMAGE_TAG_OWNER ?= perconalab
IMAGE_TAG_BASE ?= $(IMAGE_TAG_OWNER)/$(NAME)
SED := $(shell which gsed || which sed)
VERSION ?= $(shell git rev-parse --abbrev-ref HEAD | $(SED) -e 's^/^-^g; s^[.]^-^g;' | tr '[:upper:]' '[:lower:]')
IMAGE ?= $(IMAGE_TAG_BASE):$(VERSION)
DEPLOYDIR = ./deploy

BUNDLEDIR = $(DEPLOYDIR)/csv/redhat
BUNDLE_CHANNELS := --channels=stable
BUNDLE_DEFAULT_CHANNEL := --default-channel=stable
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.23
.DEFAULT_GOAL := help
.SUFFIXES:

CONTAINER ?= docker
OPENSHIFT_VERSIONS ?= v4.12-v4.15
PACKAGE_CHANNEL ?= stable
MIN_KUBE_VERSION ?= 1.24.0
DOCKER_DEFAULT_PLATFORM ?= linux/amd64
SHELL := /bin/bash
REPO_ROOT = $(shell git rev-parse --show-toplevel)
distros = community

export VERSION
export BUNDLE_REPO
export OPENSHIFT_VERSIONS
export PACKAGE_CHANNEL
export MIN_KUBE_VERSION
export DOCKER_DEFAULT_PLATFORM

REPO_ROOT = $(shell git rev-parse --show-toplevel)

distros = community

check-version:
ifndef VERSION
	$(error VERSION is not set)
endif

KUSTOMIZE = $(REPO_ROOT)/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@v4.5.3)

.PHONY: bundles
bundles: ## Build OLM bundles
bundles: check-version $(distros:%=bundles/%)

# https://olm.operatorframework.io/docs/tasks/creating-operator-bundle/#validating-your-bundle
# https://github.com/operator-framework/community-operators/blob/8a36a33/docs/packaging-required-criteria-ocp.md
.PHONY: bundles/community
bundles/community:
	cd ../../config/manager/default/ && $(KUSTOMIZE) edit set image pxc-operator=$(IMAGE)
	./generate.sh community
	env operator-sdk bundle validate $@ --select-optional='suite=operatorframework'
	env operator-sdk bundle validate $@ --select-optional='name=community' --optional-values='index-path=$@/Dockerfile'

.PHONY: clean
clean: clean-deprecated
clean: ## Remove generated files and downloaded tools
	rm -rf ./bundles ./projects ./tools ./config/marketplace

.PHONY: clean-deprecated
clean-deprecated:
	rm -rf ./package

.PHONY: help
help: ALIGN=18
help: ## Print this message
	@awk -F ': ## ' -- "/^[^':]+: ## /"' { printf "'$$(tput bold)'%-$(ALIGN)s'$$(tput sgr0)' %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: install-olm
install-olm: ## Install OLM in Kubernetes
	env operator-sdk olm install

.PHONY: tools
tools: ## Download tools needed to build bundles

tools: tools/$(SYSTEM)/jq
tools/$(SYSTEM)/jq:
	install -d '$(dir $@)'
	curl -fSL -o '$@' "https://github.com/stedolan/jq/releases/download/jq-1.7.1/jq-$$(SYSTEM='$(SYSTEM)'; \
		case "$$SYSTEM" in \
			(linux-*) echo "$${SYSTEM/-amd/}";; (darwin-*) echo "$${SYSTEM/darwin-*/osx-amd64}";; (*) echo '$(SYSTEM)';; \
		esac)"
	chmod u+x '$@'

tools: tools/$(SYSTEM)/kubectl
tools/$(SYSTEM)/kubectl:
	install -d '$(dir $@)'
	curl -fSL -o '$@' 'https://dl.k8s.io/release/$(shell curl -Ls https://dl.k8s.io/release/stable-1.21.txt)/bin/$(OS_KERNEL)/$(OS_MACHINE)/kubectl'
	chmod u+x '$@'

# quay.io/operator-framework/operator-sdk
tools: tools/$(SYSTEM)/operator-sdk
tools/$(SYSTEM)/operator-sdk:
	install -d '$(dir $@)'
	curl -fSL -o '$@' 'https://github.com/operator-framework/operator-sdk/releases/download/v1.19.1/operator-sdk_$(OS_KERNEL)_$(OS_MACHINE)'
	chmod u+x '$@'

tools: tools/$(SYSTEM)/opm
tools/$(SYSTEM)/opm:
	install -d '$(dir $@)'
	curl -fSL -o '$@' 'https://github.com/operator-framework/operator-registry/releases/download/v1.33.0/$(OS_KERNEL)-$(OS_MACHINE)-opm'
	chmod u+x '$@'

tools/$(SYSTEM)/venv:
	install -d '$(dir $@)'
	python3 -m venv '$@'

tools: tools/$(SYSTEM)/yq
tools/$(SYSTEM)/yq: | tools/$(SYSTEM)/venv
	'tools/$(SYSTEM)/venv/bin/python' -m pip install yq
	cd '$(dir $@)' && ln -s venv/bin/yq

.PHONY: validate-bundles
validate-bundles: ## Build temporary bundle images and run scorecard tests in Kubernetes
validate-bundles: $(distros:%=validate-%-image)
validate-bundles: $(distros:%=validate-%-directory)

validate-%-directory:
	./validate-directory.sh 'bundles/$*'

validate-%-image:
	./validate-image.sh '$(CONTAINER)' 'bundles/$*'

.PHONY: build-bundle-images
build-bundle-images: check-version $(distros:%=build-%-image)

build-%-image:
	./build-image.sh '$(CONTAINER)' 'bundles/$*' '$*' '$(VERSION)'
