# ==============================================================================
# Percona XtraDB Cluster Operator - OLM Bundle Generation
# ==============================================================================

# Default target
.DEFAULT_GOAL := help
.SUFFIXES:
SHELL := /bin/bash

# ==============================================================================
# Configuration Variables
# ==============================================================================

# Project configuration
NAME ?= percona-xtradb-cluster-operator
IMAGE_TAG_OWNER ?= perconalab
IMAGE_TAG_BASE ?= $(IMAGE_TAG_OWNER)/$(NAME)
MODE ?= namespace

# Version detection
SED := $(shell which gsed || which sed)
VERSION ?= $(shell git rev-parse --abbrev-ref HEAD | $(SED) -e 's^/^-^g; s^[.]^-^g;' | tr '[:upper:]' '[:lower:]')
IMAGE := $(IMAGE_TAG_BASE):$(VERSION)

# Bundle configuration
OPENSHIFT_VERSIONS ?= v4.16-v4.19
PACKAGE_CHANNEL ?= stable
MIN_KUBE_VERSION ?= ""
DOCKER_DEFAULT_PLATFORM ?= linux/amd64

# Paths
REPO_ROOT := $(shell git rev-parse --show-toplevel)
KUSTOMIZE := $(REPO_ROOT)/bin/kustomize

# Tool versions
OPERATOR_SDK_VERSION := v1.41.1

# Bundle image configuration
BUNDLE_IMG ?= $(IMAGE_TAG_BASE):community-bundle-$(VERSION)

# System detection for tool downloads
UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)
OS_KERNEL := $(shell echo "$(UNAME_S)" | tr '[:upper:]' '[:lower:]')
OS_MACHINE := $(UNAME_M)

# Display colors
GREEN := $(shell tput setaf 2)
RESET := $(shell tput sgr0)

# Export variables for generate.sh
export VERSION OPENSHIFT_VERSIONS PACKAGE_CHANNEL MIN_KUBE_VERSION DOCKER_DEFAULT_PLATFORM MODE

# ==============================================================================
# Bundle Targets
# ==============================================================================

DISTROS := community redhat marketplace

.PHONY: bundles
bundles: ## Build all OLM bundles (community, redhat, marketplace)
bundles: check-prereqs $(DISTROS:%=bundles/%)

.PHONY: $(DISTROS:%=bundles/%)
$(DISTROS:%=bundles/%): bundles/%: tools/operator-sdk
	@echo "$(GREEN)Building $* bundle...$(RESET)"
	cd ../../config/manager/$(MODE)/ && $(KUSTOMIZE) edit set image pxc-operator=$(IMAGE)
	./generate.sh $*
	./tools/operator-sdk bundle validate $@ --select-optional='suite=operatorframework'
	$(if $(filter community,$*),./tools/operator-sdk bundle validate $@ --select-optional='name=community' --optional-values='index-path=$@/Dockerfile')
	@echo "$(GREEN)✓ Bundle stored in installers/olm/bundles/$*$(RESET)"

# ==============================================================================
# Docker Build & Push Targets
# ==============================================================================

.PHONY: build
build: ## Build community bundle Docker image
build:
	@echo "$(GREEN)Building bundle Docker image...$(RESET)"
	docker build -f bundles/community/Dockerfile -t $(BUNDLE_IMG) --platform=linux/amd64 bundles/community
	@echo "$(GREEN)✓ Bundle image built: $(BUNDLE_IMG)$(RESET)"

.PHONY: push
push: ## Push bundle Docker image to registry
	@echo "$(GREEN)Pushing bundle image to registry...$(RESET)"
	docker push $(BUNDLE_IMG)
	@echo "$(GREEN)✓ Bundle image pushed: $(BUNDLE_IMG)$(RESET)"

# ==============================================================================
# Utility Targets
# ==============================================================================

.PHONY: check-prereqs
check-prereqs: check-version check-git check-tools

.PHONY: check-version
check-version:
ifndef VERSION
	$(error VERSION is not set)
endif

.PHONY: check-git
check-git:
	@if ! git rev-parse --git-dir > /dev/null 2>&1; then \
		echo "Error: Not in a git repository"; \
		exit 1; \
	fi

.PHONY: check-tools
check-tools:
	@for cmd in gawk gcsplit yq; do \
		if ! command -v $$cmd >/dev/null 2>&1; then \
			echo "Error: $$cmd is required but not installed"; \
			exit 1; \
		fi; \
	done

.PHONY: install-olm
install-olm: ## Install OLM in Kubernetes cluster
install-olm: tools/operator-sdk
	./tools/operator-sdk olm install

.PHONY: clean
clean: ## Remove generated files and downloaded tools
	rm -rf ./bundles ./projects ./tools ./package

.PHONY: help
help: ## Show this help message
	@awk 'BEGIN {FS = ": ## "; printf "\n$(GREEN)Usage:$(RESET)\n  make [target]\n\n$(GREEN)Targets:$(RESET)\n"} /^[a-zA-Z_-]+: ## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# ==============================================================================
# Tool Management
# ==============================================================================

.PHONY: tools
tools: ## Download required tools
tools: tools/operator-sdk

# Download operator-sdk
tools/operator-sdk:
	@echo "Downloading operator-sdk $(OPERATOR_SDK_VERSION)..."
	@install -d tools
	@curl -fSL --fail -o '$@' \
		'https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$(OS_KERNEL)_$(OS_MACHINE)' \
		|| { rm -f '$@'; echo "Failed to download operator-sdk"; exit 1; }
	@chmod +x '$@'
	@echo "✓ operator-sdk installed"

# ==============================================================================
# Development Targets
# ==============================================================================

.PHONY: validate
validate: ## Validate existing bundles without rebuilding
	@for distro in $(DISTROS); do \
		if [ -d "bundles/$$distro" ]; then \
			echo "Validating $$distro bundle..."; \
			./tools/operator-sdk bundle validate "bundles/$$distro" --select-optional='suite=operatorframework' || exit 1; \
		fi; \
	done
	@echo "$(GREEN)✓ All bundles validated$(RESET)"

.PHONY: list-versions
list-versions: ## Show current version information
	@echo "Current configuration:"
	@echo "  VERSION: $(VERSION)"
	@echo "  IMAGE: $(IMAGE)"
	@echo "  MODE: $(MODE)"
	@echo "  OPENSHIFT_VERSIONS: $(OPENSHIFT_VERSIONS)"
	@echo "  MIN_KUBE_VERSION: $(MIN_KUBE_VERSION)"

# ==============================================================================
# Kustomize Integration (from root Makefile)
# ==============================================================================

# Include go-get-tool function from root Makefile if kustomize target is needed
ifneq (,$(findstring kustomize,$(MAKECMDGOALS)))
include ../../Makefile
endif

.PHONY: kustomize
kustomize: ## Download kustomize locally if necessary
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@latest)
