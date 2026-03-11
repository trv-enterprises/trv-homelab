# EdgeLake Kubernetes Operator (Go)
# Makefile for building and deploying the operator

# Image configuration
IMG ?= edgelake-operator:latest
REGISTRY ?= docker.io/yourorg

# Go parameters
GOOS ?= linux
GOARCH ?= amd64

# Kubernetes configuration
NAMESPACE ?= edgelake-operator-system

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code
	go vet ./...

.PHONY: test
test: fmt vet ## Run tests
	go test ./... -coverprofile cover.out

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run

.PHONY: generate
generate: ## Generate code (deep copy, etc.)
	go generate ./...

.PHONY: manifests
manifests: ## Generate CRD manifests
	@echo "CRD manifests are in config/crd/"

##@ Build

.PHONY: build
build: fmt vet ## Build manager binary
	go build -o bin/manager cmd/manager/main.go

.PHONY: run
run: fmt vet ## Run a controller from your host
	go run ./cmd/manager/main.go

.PHONY: docker-build
docker-build: ## Build docker image
	docker build -t $(IMG) .

.PHONY: docker-push
docker-push: ## Push docker image
	docker push $(IMG)

.PHONY: docker-build-push
docker-build-push: docker-build docker-push ## Build and push docker image

##@ Deployment

.PHONY: install-crd
install-crd: ## Install CRDs into the cluster
	kubectl apply -f config/crd/

.PHONY: uninstall-crd
uninstall-crd: ## Uninstall CRDs from the cluster
	kubectl delete -f config/crd/

.PHONY: deploy
deploy: ## Deploy operator to the cluster
	@echo "Creating namespace and deploying RBAC..."
	kubectl apply -f config/manager/manager.yaml
	kubectl apply -f config/rbac/
	@echo "Operator deployed successfully"

.PHONY: undeploy
undeploy: ## Undeploy operator from the cluster
	kubectl delete -f config/rbac/ --ignore-not-found=true
	kubectl delete -f config/manager/manager.yaml --ignore-not-found=true

.PHONY: install
install: install-crd deploy ## Install CRDs and deploy operator

.PHONY: uninstall
uninstall: undeploy uninstall-crd ## Uninstall CRDs and undeploy operator

##@ Samples

.PHONY: deploy-sample-basic
deploy-sample-basic: ## Deploy basic sample EdgeLakeOperator
	kubectl apply -f config/samples/edgelake_v1alpha1_basic.yaml

.PHONY: deploy-sample-production
deploy-sample-production: ## Deploy production sample EdgeLakeOperator
	kubectl create namespace edgelake --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -f config/samples/edgelake_v1alpha1_production.yaml

.PHONY: deploy-sample-mqtt
deploy-sample-mqtt: ## Deploy MQTT sample EdgeLakeOperator
	kubectl create namespace iot-data --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -f config/samples/edgelake_v1alpha1_mqtt.yaml

.PHONY: delete-samples
delete-samples: ## Delete all sample EdgeLakeOperators
	kubectl delete -f config/samples/ --ignore-not-found=true

##@ Utilities

.PHONY: logs
logs: ## Show operator logs
	kubectl logs -n $(NAMESPACE) -l control-plane=controller-manager -f

.PHONY: status
status: ## Show operator and CR status
	@echo "=== Operator Deployment ==="
	kubectl get deployment -n $(NAMESPACE) edgelake-operator-controller-manager || true
	@echo ""
	@echo "=== Operator Pods ==="
	kubectl get pods -n $(NAMESPACE) -l control-plane=controller-manager || true
	@echo ""
	@echo "=== EdgeLakeOperator CRs ==="
	kubectl get edgelakeoperators --all-namespaces || true

.PHONY: describe-cr
describe-cr: ## Describe EdgeLakeOperator CR (NAME and CR_NAMESPACE required)
ifndef NAME
	$(error NAME is not set. Usage: make describe-cr NAME=<cr-name> CR_NAMESPACE=<namespace>)
endif
	kubectl describe edgelakeoperator $(NAME) -n $(or $(CR_NAMESPACE),default)

.PHONY: clean
clean: ## Clean build artifacts
	rm -rf bin/
	rm -f cover.out

##@ Dependencies

.PHONY: deps
deps: ## Download dependencies
	go mod download

.PHONY: tidy
tidy: ## Tidy go modules
	go mod tidy

.PHONY: vendor
vendor: ## Vendor dependencies
	go mod vendor
