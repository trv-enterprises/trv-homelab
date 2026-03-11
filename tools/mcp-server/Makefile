# EdgeLake MCP Server Build and Deployment Makefile
# Usage:
#   make build arch=arm64   - Build locally for arm64
#   make build arch=amd64   - Build on remote VM for amd64
#   make deploy             - Deploy to deployment host
#
# Parameters:
#   arch=arm64|amd64       - Target architecture (controls build location and deployment host)
#
# Can be set via environment variable: ARCH

# ====================
# NODE NAME TO REST PORT MAPPINGS
# ====================

# Define the mapping of NODE_NAME to REST_PORT
# Format: NODE_NAME=REST_PORT
NODE_PORT_MAPPINGS := \
	query=32349 \
	operator=32149 \
	operator2=32159 \
	master=32049

# ====================
# COMMAND TYPE PARSING
# ====================
# Parse command targets for optional EDGELAKE_TYPE argument
# Format: make command type (e.g., make attach query, make deploy operator2)
# Supported types: query, operator, operator2, master
# Default: query

# Default EDGELAKE_TYPE
EDGELAKE_TYPE ?= query

# Check if one of the node types is specified as a goal
ifneq ($(filter query operator operator2 master,$(MAKECMDGOALS)),)
    # Override EDGELAKE_TYPE with the specified type
    EDGELAKE_TYPE := $(firstword $(filter query operator operator2 master,$(MAKECMDGOALS)))
endif

# Map EDGELAKE_TYPE to NODE_TYPE
ifeq ($(EDGELAKE_TYPE),query)
    NODE_TYPE := query
else ifeq ($(EDGELAKE_TYPE),operator)
    NODE_TYPE := operator
else ifeq ($(EDGELAKE_TYPE),operator2)
    NODE_TYPE := operator
else ifeq ($(EDGELAKE_TYPE),master)
    NODE_TYPE := master
endif

# Set NODE_NAME based on EDGELAKE_TYPE
NODE_NAME ?= edgelake-$(EDGELAKE_TYPE)

# ====================
# HOST CONFIGURATIONS
# ====================

# Build VM (s1-build) for amd64 builds
AMD64_BUILD_USER := <user>
AMD64_BUILD_HOST := <hub-tailscale-ip>
AMD64_BUILD_PATH := /home/<user>/EdgeLake/EdgeLake

# Deployment VMs (Tailscale addresses)
DEPLOY_USER_ARM64 := edgelake
DEPLOY_USER_AMD64 := <user>
DEPLOY_HOST_ARM64 := <mcp-tailscale-ip>   # mac address: 08002721A025
DEPLOY_HOST_AMD64 := <hub-tailscale-ip>   # using host / not VM = mac address: 080027021A02
DEPLOY_PATH := /home/<user>/EdgeLake/docker-compose

# Local paths
deploy-run := /path/to/EdgeLake
DOCKER_COMPOSE_SOURCE := /path/to/docker-compose
EDGELAKE_SOURCE := /path/to/Edgelake

# Docker image configuration
REGISTRY ?= <hub-tailscale-ip>:5000
IMAGE ?= edgelake-mcp
TAG ?= latest
REGISTRY_IMAGE := $(REGISTRY)/$(IMAGE):$(TAG)

# Function to extract REST_PORT from NODE_PORT_MAPPINGS based on NODE_NAME
define get_rest_port
$(strip $(foreach mapping,$(NODE_PORT_MAPPINGS),\
	$(if $(filter $(EDGELAKE_TYPE)=%,$(mapping)),\
		$(lastword $(subst =, ,$(mapping))))))
endef

# Set REST_PORT based on EDGELAKE_TYPE mapping, or use default if not found
REST_PORT_FROM_MAP := $(call get_rest_port)
REST_PORT ?= $(if $(REST_PORT_FROM_MAP),$(REST_PORT_FROM_MAP),32349)

# ====================
# PARAMETER RESOLUTION
# ====================

# Allow environment variable to set default arch
arch ?= $(ARCH)

# Default to arm64 if arch not specified
ifeq ($(arch),)
    arch := arm64
endif

# Validate arch parameter
ifneq ($(arch),arm64)
    ifneq ($(arch),amd64)
        $(error Invalid arch value: $(arch). Use arch=arm64 or arch=amd64)
    endif
endif

# Set DEPLOY_HOST and DEPLOY_USER based on architecture
ifeq ($(arch),arm64)
    DEPLOY_HOST := $(strip $(DEPLOY_HOST_ARM64))
    DEPLOY_USER := $(strip $(DEPLOY_USER_ARM64))
else
    DEPLOY_HOST := $(strip $(DEPLOY_HOST_AMD64))
    DEPLOY_USER := $(strip $(DEPLOY_USER_AMD64))
endif

# Set architecture-specific TAG if using default
ifeq ($(TAG),latest)
    TAG := $(arch)-latest
endif

# Update REGISTRY_IMAGE with architecture-specific TAG
REGISTRY_IMAGE := $(REGISTRY)/$(IMAGE):$(TAG)

# ====================
# BUILD TARGETS
# ====================

.PHONY: build build-arm64 build-amd64

build:
	@echo "=== Architecture: $(arch) ==="
ifeq ($(arch),arm64)
	@$(MAKE) build-arm64 arch=$(arch)
else ifeq ($(arch),amd64)
	@$(MAKE) build-amd64 arch=$(arch)
endif

build-arm64:
	@echo "Building $(IMAGE):$(TAG) locally for arm64..."
	@cd $(EDGELAKE_SOURCE) && docker build -t $(IMAGE):$(TAG) .
	@echo "Tagging image for registry: $(REGISTRY_IMAGE)"
	@docker tag $(IMAGE):$(TAG) $(REGISTRY_IMAGE)
	@echo "Pushing to registry: $(REGISTRY_IMAGE)"
	@docker push $(REGISTRY_IMAGE)
	@echo "Pulling image to deployment host..."
	@$(MAKE) pull-image arch=arm64

build-amd64:
	@echo "Syncing EdgeLake source to $(AMD64_BUILD_USER)@$(AMD64_BUILD_HOST):$(AMD64_BUILD_PATH)..."
	@rsync -avz --progress --exclude='.git' --exclude='*.pyc' --exclude='__pycache__' \
		$(EDGELAKE_SOURCE)/ $(AMD64_BUILD_USER)@$(AMD64_BUILD_HOST):$(AMD64_BUILD_PATH)/
	@echo "Building $(IMAGE):$(TAG) on remote VM for amd64..."
	@ssh $(AMD64_BUILD_USER)@$(AMD64_BUILD_HOST) \
		"cd $(AMD64_BUILD_PATH) && docker build -t $(IMAGE):$(TAG) ."
	@echo "Tagging and pushing to registry from build VM: $(REGISTRY_IMAGE)"
	@ssh $(AMD64_BUILD_USER)@$(AMD64_BUILD_HOST) \
		"docker tag $(IMAGE):$(TAG) $(REGISTRY_IMAGE) && docker push $(REGISTRY_IMAGE)"
	@echo "Pulling image to deployment host..."
	@$(MAKE) pull-image arch=amd64

# ====================
# DEPLOY TARGETS
# ====================

.PHONY: all deploy clean-node clean-vols clear-vols down push pull-image sync up up-all down-all clean-all-vols clear-all-vols clear-vols-all clean-vols-all start-all stop-all

# Complete workflow: build and deploy
all:
	@echo "=== Complete Build and Deploy Workflow (arch=$(arch)) ==="
	@$(MAKE) build arch=$(arch)
	@$(MAKE) deploy arch=$(arch)

deploy:
	@echo "=== Deploying to $(DEPLOY_HOST) (arch=$(arch)) ==="
	@$(MAKE) down clean-vols sync up

# Clean deployment - removes container, volumes, and image
# Use this when you need to completely reset the node (removes all data/config/scripts)
# WARNING: This removes ALL data for this node (blockchain, data, scripts)
clean-node:
	@echo "=== Cleaning Entire Node , including image on $(DEPLOY_HOST) ==="
	@echo "Cleaning $(EDGELAKE_TYPE) container, volumes, and images on $(DEPLOY_HOST)..."
	@ssh $(DEPLOY_USER)@$(DEPLOY_HOST) \
		"cd $(DEPLOY_PATH) && make clean EDGELAKE_TYPE=$(EDGELAKE_TYPE)"

clean-vols:
	@echo "=== Cleaning Volumes on $(DEPLOY_HOST) ==="
	@echo "Cleaning $(EDGELAKE_TYPE)  volumes on $(DEPLOY_HOST)..."
	@ssh $(DEPLOY_USER)@$(DEPLOY_HOST) \
		"cd $(DEPLOY_PATH) && make clean-vols EDGELAKE_TYPE=$(EDGELAKE_TYPE)"

down:
	@echo "Stopping existing $(EDGELAKE_TYPE) container on $(DEPLOY_HOST)...."
	@ssh $(DEPLOY_USER)@$(DEPLOY_HOST) \
		"cd $(DEPLOY_PATH) && make down EDGELAKE_TYPE=$(EDGELAKE_TYPE)"

# Pull image from registry to deployment host
pull-image:
	@echo "Pulling $(REGISTRY_IMAGE) on $(DEPLOY_HOST)..."
	@ssh $(DEPLOY_USER)@$(DEPLOY_HOST) "docker pull $(REGISTRY_IMAGE)"
	@echo "Tagging as $(IMAGE):$(TAG) on $(DEPLOY_HOST)..."
	@ssh $(DEPLOY_USER)@$(DEPLOY_HOST) "docker tag $(REGISTRY_IMAGE) $(IMAGE):$(TAG)"

# Legacy tar-based push (deprecated - use pull-image instead)
push:
	@echo "Pushing $(IMAGE):$(TAG) to $(DEPLOY_USER)@$(DEPLOY_HOST)..."
	@echo "Saving image to temporary file..."
	@docker save $(IMAGE):$(TAG) -o /tmp/$(IMAGE)-$(TAG).tar
	@echo "Transferring image..."
	@rsync -avz --progress /tmp/$(IMAGE)-$(TAG).tar $(DEPLOY_USER)@$(DEPLOY_HOST):~/
	@echo "Loading image on remote host..."
	@ssh $(DEPLOY_USER)@$(DEPLOY_HOST) "docker load -i ~/$(IMAGE)-$(TAG).tar && rm ~/$(IMAGE)-$(TAG).tar"
	@rm /tmp/$(IMAGE)-$(TAG).tar

sync:
	@echo "Syncing docker-compose to $(DEPLOY_USER)@$(DEPLOY_HOST):$(DEPLOY_PATH)..."
	@rsync -avz --delete $(DOCKER_COMPOSE_SOURCE)/ $(DEPLOY_USER)@$(DEPLOY_HOST):$(DEPLOY_PATH)/

up:
	@echo "Running 'make up' on $(DEPLOY_HOST)...$(IMAGE):$(TAG)"
	@ssh $(DEPLOY_USER)@$(DEPLOY_HOST) \
		"cd $(DEPLOY_PATH) && make up EDGELAKE_TYPE=$(EDGELAKE_TYPE) TAG=$(TAG)"

up-all:
	@echo "Starting all EdgeLake nodes on $(DEPLOY_HOST)..."
	@ssh $(DEPLOY_USER)@$(DEPLOY_HOST) \
		"cd $(DEPLOY_PATH) && \
		make up EDGELAKE_TYPE=master TAG=$(TAG) && \
		echo 'Waiting 10 seconds for master to start...' && \
		sleep 10 && \
		make up EDGELAKE_TYPE=operator TAG=$(TAG) && \
		make up EDGELAKE_TYPE=operator2 TAG=$(TAG) && \
		sleep 10 && \
		make up EDGELAKE_TYPE=query TAG=$(TAG) && \
		echo 'All nodes started successfully'"

down-all:
	@echo "Stopping all EdgeLake nodes on $(DEPLOY_HOST)..."
	@ssh $(DEPLOY_USER)@$(DEPLOY_HOST) \
		"cd $(DEPLOY_PATH) && \
		make down NODE_NAME=edgelake-query && \
		make down NODE_NAME=edgelake-operator2 && \
		make down NODE_NAME=edgelake-operator && \
		make down NODE_NAME=edgelake-master && \
		echo 'All nodes stopped successfully'"

clean-all-vols:
	@echo "Cleaning all EdgeLake volumes on $(DEPLOY_HOST)..."
	@ssh $(DEPLOY_USER)@$(DEPLOY_HOST) \
		"cd $(DEPLOY_PATH) && \
		make clean-vols EDGELAKE_TYPE=query && \
		make clean-vols EDGELAKE_TYPE=operator2 && \
		make clean-vols EDGELAKE_TYPE=operator && \
		make clean-vols EDGELAKE_TYPE=master && \
		echo 'All nodes cleaned successfully'"

# Synonyms for clean-vols
clear-vols: clean-vols

# Synonyms for clean-all-vols
clear-all-vols: clean-all-vols
clear-vols-all: clean-all-vols
clean-vols-all: clean-all-vols

# Synonyms for up-all and down-all
start-all: up-all
stop-all: down-all


# ====================
# UTILITY TARGETS
# ====================

.PHONY: push-registry test-mcp-up test-mcp-tools ssh ssh-copy connect attach logs start-mcp-server get-echo-queue info get-local-ip load-data docker-status

# Push locally built image to registry (manual operation if needed)
push-registry:
	@echo "Tagging and pushing $(IMAGE):$(TAG) to registry..."
	@docker tag $(IMAGE):$(TAG) $(REGISTRY_IMAGE)
	@docker push $(REGISTRY_IMAGE)

# Test if MCP endpoint is responding
test-mcp-up:
	@echo "Testing MCP endpoint: http://$(DEPLOY_HOST):32349/mcp/sse"
	@curl -v http://$(DEPLOY_HOST):32349/mcp/sse

# Test MCP tools
test-mcp-tools:
	@echo "Testing MCP tools on $(DEPLOY_HOST):32349..."
	@python3 $(EDGELAKE_SOURCE)/test-mcp-sse.py --host $(DEPLOY_HOST) --port 32349

# SSH to deployment host
ssh:
	@echo "Connecting to $(DEPLOY_USER)@$(DEPLOY_HOST)..."
	@ssh $(DEPLOY_USER)@$(DEPLOY_HOST)

# Copy SSH key to deployment host
ssh-copy:
	@echo "Copying SSH key to $(DEPLOY_USER)@$(DEPLOY_HOST)..."
	@ssh-copy-id $(DEPLOY_USER)@$(DEPLOY_HOST)

# Connect to container on deployment host
connect:
	@echo "Connecting to edgelake-$(EDGELAKE_TYPE) on $(DEPLOY_HOST)..."
	@ssh -t $(DEPLOY_USER)@$(DEPLOY_HOST) "docker exec -it $(NODE_NAME) bash"

# Attach to container (EdgeLake CLI)
attach:
	@echo "Attaching to edgelake-$(EDGELAKE_TYPE) on $(DEPLOY_HOST)..."
	@echo "Use Ctrl-D to detach"
	@ssh -t $(DEPLOY_USER)@$(DEPLOY_HOST) "docker attach --detach-keys=ctrl-d $(NODE_NAME)"

# Show container logs
logs:
	@echo "Showing logs for edgelake-$(EDGELAKE_TYPE) on $(DEPLOY_HOST)..."
	@ssh $(DEPLOY_USER)@$(DEPLOY_HOST) "docker logs $(NODE_NAME)"

# Run MCP server on deployment host
run-mcp-server:
	@echo "Running MCP server on $(DEPLOY_HOST):$(REST_PORT)..."
	@curl -X POST http://$(DEPLOY_HOST):$(REST_PORT) \
		-H "Content-Type: text/plain" \
		-H "command: run mcp server" \
		-H "User-Agent: AnyLog/1.23"

# Get echo queue from deployment host
get-echo-queue:
	@echo "Getting echo queue from $(DEPLOY_HOST):$(REST_PORT)..."
	@curl -X GET http://$(DEPLOY_HOST):$(REST_PORT) \
		-H "Content-Type: text/plain" \
		-H "command: get echo queue" \
		-H "User-Agent: AnyLog/1.23"

# Show configuration info
info:
	@echo "=== EdgeLake MCP Makefile Configuration ==="
	@echo ""
	@echo "Target Architecture:         $(arch)"
	@echo ""
	@echo "Build Configuration:"
ifeq ($(arch),arm64)
	@echo "  Build Location:            Local (this machine)"
	@echo "  Source Path:               $(EDGELAKE_SOURCE)"
else
	@echo "  Build Location:            Remote VM (amd64)"
	@echo "  Remote Build Host:         $(AMD64_BUILD_HOST)"
	@echo "  Remote Build User:         $(AMD64_BUILD_USER)"
	@echo "  Remote Build Path:         $(AMD64_BUILD_PATH)"
endif
	@echo ""
	@echo "Deploy Configuration:"
	@echo "  Deploy Host:               $(DEPLOY_HOST)"
	@echo "  Deploy User:               $(DEPLOY_USER)"
	@echo "  Deploy Path:               $(DEPLOY_PATH)"
	@echo ""
	@echo "Docker Configuration:"
	@echo "  Registry:                  $(REGISTRY)"
	@echo "  Image:                     $(IMAGE)"
	@echo "  Tag:                       $(TAG)"
	@echo "  Registry Image:            $(REGISTRY_IMAGE)"
	@echo "  EdgeLake Type:             $(EDGELAKE_TYPE)"
	@echo "  Node Name:                 $(NODE_NAME)"
	@echo "  REST Port:                 $(REST_PORT) $(if $(REST_PORT_FROM_MAP),(from mapping),)"
	@echo ""
	@echo "Usage Examples:"
	@echo "  make build arch=arm64                  - Build locally for arm64"
	@echo "  make build arch=amd64                  - Build on remote VM for amd64"
	@echo "  make deploy                            - Deploy query node (default)"
	@echo "  make deploy operator2                  - Deploy operator2 node"
	@echo "  make attach query                      - Attach to query node CLI"
	@echo "  make logs operator                     - Show logs for operator node"
	@echo "  make info                              - Show current configuration"
	@echo "  make info master                       - Show config for master node"
	@echo ""
	@echo "Command Type Syntax:"
	@echo "  Format: make command [type]"
	@echo "  Supported types: query, operator, operator2, master (default: query)"
	@echo "  Commands: attach, deploy, connect, logs, run-mcp-server,"
	@echo "            get-echo-queue, load-data, info"
	@echo ""
	@echo "Environment Variables (optional):"
	@echo "  export ARCH=arm64                      - Set default architecture"
	@echo "  export EDGELAKE_TYPE=operator          - Set default EdgeLake type"

# Get local IP address and network adapter
get-local-ip:
	@echo "=== Local Network Information ==="
	@ifconfig | awk '/^[a-z]/ { iface=$$1; sub(/:$$/, "", iface) } \
		/inet / && !/127\.0\.0\.1/ { \
			print "Network Adapter: " iface; \
			print "IP Address:      " $$2; \
			exit \
		}'

# Check Docker status on deployment host
docker-status:
	@echo "=== Docker Status on $(DEPLOY_HOST) ==="
	@echo ""
	@echo "--- Docker Containers (docker ps -a) ---"
	@ssh $(DEPLOY_USER)@$(DEPLOY_HOST) "docker ps -a"
	@echo ""
	@echo "--- Docker Volumes (docker volume ls) ---"
	@ssh $(DEPLOY_USER)@$(DEPLOY_HOST) "docker volume ls"

# Load sample data to EdgeLake operator node
# Usage: make load-data [operator|operator2] [dbms=<name>] [tables=<t1,t2>] [count=<n>] [dry-run=true]
# Parameters:
#   EDGELAKE_TYPE     - Target node type (default: operator)
#   dbms=<name>       - Database name (default: aiops)
#   tables=<t1,t2>    - Specific tables (default: all)
#   count=<n>         - Records per table (default: 10)
#   dry-run=true      - Preview data without loading
load-data:
	@echo "Discovering $(EDGELAKE_TYPE) endpoint..."
	@ENDPOINT=$$(ssh $(DEPLOY_USER)@$(DEPLOY_HOST) \
		"docker exec $(NODE_NAME) bash -c 'echo \$$OVERLAY_IP:\$$ANYLOG_REST_PORT'"); \
	HOST=$$(echo $$ENDPOINT | cut -d: -f1); \
	PORT=$$(echo $$ENDPOINT | cut -d: -f2); \
	echo "Loading sample data to $$HOST:$$PORT..."; \
	data-loader/venv/bin/python data-loader/load_sample_data_fixed.py \
		--host $$HOST \
		--port $$PORT \
		$(if $(dbms),--dbms $(dbms),) \
		$(if $(tables),--tables $(subst $(comma), ,$(tables)),) \
		$(if $(count),--count $(count),) \
		$(if $(dry-run),--dry-run,)

# ====================
# COMPLETE WORKFLOW
# ====================

# Help target
.PHONY: help
help:
	@echo "EdgeLake MCP Server Build and Deployment Makefile"
	@echo ""
	@echo "Parameters:"
	@echo "  arch=<arm64|amd64>         - Target architecture (default: arm64)"
	@echo "                               Controls build location and deployment host"
	@echo ""
	@echo "Environment Variables (optional):"
	@echo "  ARCH                       - Set default architecture"
	@echo "  EDGELAKE_TYPE              - Set default EdgeLake type"
	@echo ""
	@echo "Targets:"
	@echo "  make build [arch=<arch>]   - Build image, push to registry, pull to deployment host"
	@echo "  make deploy                - Deploy to deployment host (uses pre-pulled image)"
	@echo "  make up-all                - Start all nodes (master, operator, operator2, query)"
	@echo "  make down-all              - Stop all nodes"
	@echo "  make clean-all             - Clean all nodes (removes data and volumes)"
	@echo "  make all [arch=<arch>]     - Build and deploy"
	@echo "  make push-registry         - Manually push local image to registry"
	@echo "  make pull-image            - Pull image from registry to deployment host"
	@echo "  make test-mcp-up           - Test if MCP endpoint is responding"
	@echo "  make test-mcp-tools        - Test MCP tools"
	@echo "  make ssh                   - SSH to deployment host"
	@echo "  make ssh-copy              - Copy SSH key to deployment host"
	@echo "  make connect               - Connect to container bash shell"
	@echo "  make attach                - Attach to EdgeLake CLI (Ctrl-D to detach)"
	@echo "  make logs                  - Show container logs"
	@echo "  make run-mcp-server        - Send 'run mcp server' command to EdgeLake"
	@echo "  make get-echo-queue        - Send 'get echo queue' command to EdgeLake"
	@echo "  make get-local-ip          - Show local network adapter and IP address"
	@echo "  make docker-status         - Check Docker containers and volumes on deployment host"
	@echo "  make load-data [options]   - Load sample data to operator node"
	@echo "  make info                  - Show configuration"
	@echo "  make help                  - Show this help"
	@echo ""
	@echo "Examples:"
	@echo "  make build arch=arm64                                  - Build for arm64, push to registry, pull to deployment host"
	@echo "  make build arch=amd64                                  - Build for amd64, push to registry, pull to deployment host"
	@echo "  make deploy                                            - Deploy query node (uses pre-pulled image)"
	@echo "  make deploy operator2                                  - Deploy operator2 node"
	@echo "  make all arch=arm64                                    - Complete build and deploy"
	@echo "  make attach query                                      - Attach to query node CLI"
	@echo "  make connect operator                                  - Connect to operator bash shell"
	@echo "  make logs master                                       - Show master node logs"
	@echo "  make run-mcp-server query                              - Start MCP server on query node"
	@echo "  make get-echo-queue operator2                          - Get echo queue from operator2"
	@echo "  make load-data                                         - Load default sample data"
	@echo "  make load-data count=100                               - Load 100 records per table"
	@echo ""
	@echo "Command Type Syntax:"
	@echo "  Format: make command [type]"
	@echo "  Supported types: query, operator, operator2, master (default: query)"
	@echo "  Commands: attach, deploy, connect, logs, run-mcp-server,"
	@echo "            get-echo-queue, load-data, info"
	@echo ""
	@echo "With environment variable:"
	@echo "  export ARCH=arm64"
	@echo "  make build                                             - Uses environment default"
	@echo "  export EDGELAKE_TYPE=operator"
	@echo "  make attach                                            - Attach to operator node"

# ====================
# NODE TYPE DUMMY TARGETS
# ====================
# Dummy targets for node types so they can be used as arguments
# Example: make attach query, make deploy operator2

.PHONY: query operator operator2 master

query operator operator2 master:
	@:
