#!/usr/bin/make -f
# Open Horizon EdgeLake Kubernetes Operator Publishing Makefile
#
# This Makefile manages publishing the EdgeLake Kubernetes Operator
# to Open Horizon Exchange for deployment to edge clusters.
#
# Prerequisites:
#   - hzn CLI installed and configured
#   - Access to OH Exchange at HZN_EXCHANGE_URL
#   - Kubernetes edge cluster with OH agent installed
#   - Operator container image built and pushed to registry

SHELL := /bin/bash

# ====================
# OPEN HORIZON CONFIGURATION
# ====================

# Open Horizon organization ID
export HZN_ORG_ID ?= myorg

# Open Horizon Exchange credentials (admin:<password>)
export HZN_EXCHANGE_USER_AUTH ?= myorg/admin:changeme

# Open Horizon Exchange URL
export HZN_EXCHANGE_URL ?= http://<hub-tailscale-ip>:3090/v1

# ====================
# SERVICE CONFIGURATION
# ====================

# Service name (must match service.definition.json url field)
export SERVICE_NAME ?= edgelake-kube-operator

# Service version
export SERVICE_VERSION ?= 1.0.0

# Architecture
export ARCH ?= $(shell hzn architecture 2>/dev/null || echo "amd64")

# Operator container image
export OPERATOR_IMAGE ?= <hub-tailscale-ip>:5000/edgelake-operator:$(SERVICE_VERSION)

# ====================
# PATHS
# ====================

OH_DIR := .
OPERATOR_YAMLS_DIR := $(OH_DIR)/operator-yamls
TAR_FILE := $(OH_DIR)/operator-yamls.tar.gz
SERVICE_DEF := $(OH_DIR)/service.definition.json
SERVICE_POLICY := $(OH_DIR)/service.policy.json
DEPLOYMENT_POLICY := $(OH_DIR)/deployment.policy.json
NODE_POLICY := $(OH_DIR)/node.policy.json

# ====================
# TARGETS
# ====================

.PHONY: help
help: ## Show this help message
	@echo "==============================================="
	@echo "Open Horizon EdgeLake Kubernetes Operator"
	@echo "==============================================="
	@echo ""
	@echo "Configuration:"
	@echo "  HZN_ORG_ID:        $(HZN_ORG_ID)"
	@echo "  HZN_EXCHANGE_URL:  $(HZN_EXCHANGE_URL)"
	@echo "  SERVICE_NAME:      $(SERVICE_NAME)"
	@echo "  SERVICE_VERSION:   $(SERVICE_VERSION)"
	@echo "  OPERATOR_IMAGE:    $(OPERATOR_IMAGE)"
	@echo "  ARCH:              $(ARCH)"
	@echo ""
	@echo "Commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2}'

# ====================
# BUILD
# ====================

.PHONY: build-tar
build-tar: ## Create operator YAML tar.gz archive
	@echo "======================================="
	@echo "Creating Operator YAML Archive"
	@echo "======================================="
	@echo "Substituting OPERATOR_IMAGE in deployment..."
	@sed -i.bak 's|$${OPERATOR_IMAGE}|$(OPERATOR_IMAGE)|g' $(OPERATOR_YAMLS_DIR)/05-operator-deployment.yaml
	@rm -f $(OPERATOR_YAMLS_DIR)/05-operator-deployment.yaml.bak
	@echo "Creating tar.gz archive..."
	@cd $(OPERATOR_YAMLS_DIR) && tar --exclude='._*' --exclude='.DS_Store' -cvzf ../operator-yamls.tar.gz *.yaml
	@echo "✓ Archive created: $(TAR_FILE)"
	@ls -la $(TAR_FILE)

.PHONY: build-service-def
build-service-def: ## Generate service definition with substitutions
	@echo "======================================="
	@echo "Generating Service Definition"
	@echo "======================================="
	@if [ ! -f "$(SERVICE_DEF)" ]; then \
		echo "❌ Service definition not found: $(SERVICE_DEF)"; \
		exit 1; \
	fi
	@envsubst < $(SERVICE_DEF) > $(SERVICE_DEF).tmp
	@mv $(SERVICE_DEF).tmp $(SERVICE_DEF).generated
	@echo "✓ Service definition generated: $(SERVICE_DEF).generated"

.PHONY: build
build: build-tar build-service-def ## Build all artifacts (tar + service def)
	@echo ""
	@echo "✓ Build complete"
	@echo "  - Tar archive: $(TAR_FILE)"
	@echo "  - Service def: $(SERVICE_DEF).generated"

# ====================
# PUBLISHING
# ====================

.PHONY: publish-service
publish-service: build check-hzn ## Publish service to Exchange
	@echo "======================================="
	@echo "Publishing Service to Exchange"
	@echo "======================================="
	@if [ ! -f "$(TAR_FILE)" ]; then \
		echo "❌ Tar archive not found. Run 'make build-tar' first"; \
		exit 1; \
	fi
	@hzn exchange service publish \
		--org=$(HZN_ORG_ID) \
		--user-pw=$(HZN_EXCHANGE_USER_AUTH) \
		-O -P \
		--json-file=$(SERVICE_DEF).generated \
		--file=$(TAR_FILE)
	@echo "✓ Service published: $(HZN_ORG_ID)/$(SERVICE_NAME)_$(SERVICE_VERSION)_$(ARCH)"

.PHONY: publish-service-policy
publish-service-policy: check-hzn ## Publish service policy to Exchange
	@echo "======================================="
	@echo "Publishing Service Policy"
	@echo "======================================="
	@hzn exchange service addpolicy \
		--org=$(HZN_ORG_ID) \
		--user-pw=$(HZN_EXCHANGE_USER_AUTH) \
		-f $(SERVICE_POLICY) \
		$(HZN_ORG_ID)/$(SERVICE_NAME)_$(SERVICE_VERSION)_$(ARCH)
	@echo "✓ Service policy published"

.PHONY: publish-deployment
publish-deployment: check-hzn ## Publish deployment policy to Exchange
	@echo "======================================="
	@echo "Publishing Deployment Policy"
	@echo "======================================="
	@envsubst < $(DEPLOYMENT_POLICY) > $(DEPLOYMENT_POLICY).generated
	@hzn exchange deployment addpolicy \
		--org=$(HZN_ORG_ID) \
		--user-pw=$(HZN_EXCHANGE_USER_AUTH) \
		-f $(DEPLOYMENT_POLICY).generated \
		$(HZN_ORG_ID)/policy-$(SERVICE_NAME)_$(SERVICE_VERSION)
	@echo "✓ Deployment policy published: policy-$(SERVICE_NAME)_$(SERVICE_VERSION)"

.PHONY: publish-all
publish-all: publish-service publish-service-policy publish-deployment ## Publish everything
	@echo "======================================="
	@echo "All Components Published Successfully"
	@echo "======================================="
	@echo ""
	@echo "Service:    $(HZN_ORG_ID)/$(SERVICE_NAME)_$(SERVICE_VERSION)_$(ARCH)"
	@echo "Deployment: $(HZN_ORG_ID)/policy-$(SERVICE_NAME)_$(SERVICE_VERSION)"
	@echo ""
	@echo "Next steps:"
	@echo "1. On Kubernetes edge cluster, ensure OH agent is installed"
	@echo "2. Register cluster with node policy:"
	@echo "   hzn register --policy=$(NODE_POLICY)"
	@echo "3. Monitor agreement:"
	@echo "   watch hzn agreement list"
	@echo "4. Check operator deployment:"
	@echo "   kubectl get pods -n edgelake-operator-system"

# ====================
# INSPECTION
# ====================

.PHONY: list-services
list-services: check-hzn ## List published services
	@echo "======================================="
	@echo "Published Services"
	@echo "======================================="
	@hzn exchange service list \
		--org=$(HZN_ORG_ID) \
		--user-pw=$(HZN_EXCHANGE_USER_AUTH) | grep -i edgelake || echo "No EdgeLake services found"

.PHONY: list-deployments
list-deployments: check-hzn ## List deployment policies
	@echo "======================================="
	@echo "Deployment Policies"
	@echo "======================================="
	@hzn exchange deployment listpolicy \
		--org=$(HZN_ORG_ID) \
		--user-pw=$(HZN_EXCHANGE_USER_AUTH) | grep -i edgelake || echo "No EdgeLake deployment policies found"

.PHONY: show-service
show-service: check-hzn ## Show service definition details
	@hzn exchange service list \
		--org=$(HZN_ORG_ID) \
		--user-pw=$(HZN_EXCHANGE_USER_AUTH) \
		$(HZN_ORG_ID)/$(SERVICE_NAME)_$(SERVICE_VERSION)_$(ARCH)

.PHONY: show-deployment
show-deployment: check-hzn ## Show deployment policy details
	@hzn exchange deployment listpolicy \
		--org=$(HZN_ORG_ID) \
		--user-pw=$(HZN_EXCHANGE_USER_AUTH) \
		$(HZN_ORG_ID)/policy-$(SERVICE_NAME)_$(SERVICE_VERSION)

# ====================
# CLEANUP
# ====================

.PHONY: remove-service
remove-service: check-hzn ## Remove service from Exchange
	@echo "Removing service from Exchange..."
	@hzn exchange service remove \
		--org=$(HZN_ORG_ID) \
		--user-pw=$(HZN_EXCHANGE_USER_AUTH) \
		--force \
		$(HZN_ORG_ID)/$(SERVICE_NAME)_$(SERVICE_VERSION)_$(ARCH) || true
	@echo "✓ Service removed (if it existed)"

.PHONY: remove-deployment
remove-deployment: check-hzn ## Remove deployment policy from Exchange
	@echo "Removing deployment policy..."
	@hzn exchange deployment removepolicy \
		--org=$(HZN_ORG_ID) \
		--user-pw=$(HZN_EXCHANGE_USER_AUTH) \
		--force \
		$(HZN_ORG_ID)/policy-$(SERVICE_NAME)_$(SERVICE_VERSION) || true
	@echo "✓ Deployment policy removed (if it existed)"

.PHONY: clean
clean: ## Remove generated files
	@echo "Cleaning generated files..."
	@rm -f $(TAR_FILE)
	@rm -f $(SERVICE_DEF).generated
	@rm -f $(SERVICE_DEF).tmp
	@rm -f $(DEPLOYMENT_POLICY).generated
	@echo "✓ Clean complete"

# ====================
# VALIDATION
# ====================

.PHONY: check-hzn
check-hzn: ## Check hzn CLI configuration
	@if ! command -v hzn >/dev/null 2>&1; then \
		echo "❌ hzn CLI not found. Please install Open Horizon agent."; \
		exit 1; \
	fi
	@if [ -z "$(HZN_ORG_ID)" ]; then \
		echo "❌ HZN_ORG_ID not set"; \
		exit 1; \
	fi

.PHONY: check-vars
check-vars: ## Validate configuration variables
	@echo "======================================="
	@echo "Configuration Validation"
	@echo "======================================="
	@echo "HZN_ORG_ID:           $(HZN_ORG_ID)"
	@echo "HZN_EXCHANGE_URL:     $(HZN_EXCHANGE_URL)"
	@echo "HZN_EXCHANGE_USER_AUTH: $(shell echo $(HZN_EXCHANGE_USER_AUTH) | sed 's/:.*/:*****/')"
	@echo "SERVICE_NAME:         $(SERVICE_NAME)"
	@echo "SERVICE_VERSION:      $(SERVICE_VERSION)"
	@echo "OPERATOR_IMAGE:       $(OPERATOR_IMAGE)"
	@echo "ARCH:                 $(ARCH)"
	@echo ""
	@echo "Files:"
	@if [ -f "$(TAR_FILE)" ]; then echo "  ✓ $(TAR_FILE)"; else echo "  ❌ $(TAR_FILE) (run make build-tar)"; fi
	@if [ -f "$(SERVICE_DEF)" ]; then echo "  ✓ $(SERVICE_DEF)"; else echo "  ❌ $(SERVICE_DEF)"; fi
	@if [ -f "$(SERVICE_POLICY)" ]; then echo "  ✓ $(SERVICE_POLICY)"; else echo "  ❌ $(SERVICE_POLICY)"; fi
	@if [ -f "$(DEPLOYMENT_POLICY)" ]; then echo "  ✓ $(DEPLOYMENT_POLICY)"; else echo "  ❌ $(DEPLOYMENT_POLICY)"; fi
	@if [ -f "$(NODE_POLICY)" ]; then echo "  ✓ $(NODE_POLICY)"; else echo "  ❌ $(NODE_POLICY)"; fi

.PHONY: test-connection
test-connection: check-hzn ## Test connection to Exchange
	@echo "Testing Exchange connection..."
	@hzn exchange status || (echo "❌ Cannot connect to Exchange"; exit 1)
	@echo "✓ Connection successful"

# ====================
# OPERATOR BUILD
# ====================

.PHONY: docker-build-operator
docker-build-operator: ## Build operator container image
	@echo "======================================="
	@echo "Building Operator Container Image"
	@echo "======================================="
	@cd .. && docker build -t $(OPERATOR_IMAGE) .
	@echo "✓ Operator image built: $(OPERATOR_IMAGE)"

.PHONY: docker-push-operator
docker-push-operator: ## Push operator container image
	@echo "Pushing operator image..."
	@docker push $(OPERATOR_IMAGE)
	@echo "✓ Operator image pushed: $(OPERATOR_IMAGE)"

.PHONY: docker-build-push
docker-build-push: docker-build-operator docker-push-operator ## Build and push operator image

.DEFAULT_GOAL := help
