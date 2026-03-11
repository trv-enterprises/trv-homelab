# EdgeLake Docker Compose - Layered Configuration Makefile
# Usage:
#   make up master              - Start master deployment
#   make down query             - Stop query deployment
#   make attach master          - Attach to master node CLI
#   make clean query            - Clean query volumes
#
# Supported deployment types: master, query

# ====================
# ROOT CONFIGURATION (Edit this section when duplicating to new environment)
# ====================

# Load root-level configuration if it exists
# Copy .env.root.template to .env.root and customize
-include .env.root

# Docker image configuration (can be overridden per deployment in .env)
DEFAULT_IMAGE ?= edgelake-mcp
DEFAULT_TAG ?= amd64-latest

# Default deployment type if none specified
DEFAULT_EDGELAKE_TYPE ?= query

# SSH user for remote connections (can be overridden)
SSH_USER ?= $(USER)

# Remote deployment path (can be overridden)
REMOTE_DEPLOY_PATH ?= /home/$(SSH_USER)/EdgeLake/docker-compose

# Docker registry configuration (can be overridden)
REGISTRY ?= <hub-tailscale-ip>:5000

# Use sudo for docker commands if needed (set to empty string if user is in docker group)
DOCKER_SUDO ?= sudo

# ====================
# DEPLOYMENT TYPE PARSING
# ====================

# List of valid deployment types (auto-discovered from deployments/ directory)
DEPLOYMENT_TYPES := master query

# Default to DEFAULT_EDGELAKE_TYPE
EDGELAKE_TYPE ?= $(DEFAULT_EDGELAKE_TYPE)

# Check if one of the deployment types is specified as a goal
ifneq ($(filter $(DEPLOYMENT_TYPES),$(MAKECMDGOALS)),)
    EDGELAKE_TYPE := $(firstword $(filter $(DEPLOYMENT_TYPES),$(MAKECMDGOALS)))
endif

# Set deployment directory
DEPLOYMENT_DIR := deployments/$(EDGELAKE_TYPE)

# ====================
# DYNAMIC VARIABLE EXTRACTION
# ====================

# Function to extract a variable from deployment's .env and config files
# Priority: .env > advance_configs.env > base_configs.env
# Usage: $(call get-var,VARIABLE_NAME)
define get-var
$(shell grep -h "^$(1)=" \
	$(DEPLOYMENT_DIR)/.env \
	$(DEPLOYMENT_DIR)/configs/advance_configs.env \
	$(DEPLOYMENT_DIR)/configs/base_configs.env \
	2>/dev/null | head -1 | cut -d'=' -f2- | sed 's/"//g' | sed "s/'//g")
endef

# Extract key variables dynamically from deployment configs
NODE_NAME := $(call get-var,NODE_NAME)
OVERLAY_IP := $(call get-var,OVERLAY_IP)
ANYLOG_SERVER_PORT := $(call get-var,ANYLOG_SERVER_PORT)
ANYLOG_REST_PORT := $(call get-var,ANYLOG_REST_PORT)
IMAGE := $(or $(call get-var,IMAGE),$(DEFAULT_IMAGE))
TAG := $(or $(call get-var,TAG),$(DEFAULT_TAG))

# If NODE_NAME is empty, use default pattern
ifeq ($(NODE_NAME),)
    NODE_NAME := edgelake-$(EDGELAKE_TYPE)
endif

# SSH configuration
# If OVERLAY_IP is set, use it for SSH, otherwise use localhost
SSH_HOST := $(or $(OVERLAY_IP),localhost)

# ====================
# DOCKER COMPOSE COMMANDS
# ====================

# Base docker-compose command for all operations
# Export variables so they're available to docker-compose
# Each deployment has its own copy of docker-compose.base.yml for correct path resolution
COMPOSE := cd $(CURDIR)/$(DEPLOYMENT_DIR) && \
	$(DOCKER_SUDO) NODE_NAME=$(NODE_NAME) \
	IMAGE=$(IMAGE) \
	TAG=$(TAG) \
	docker-compose -f docker-compose.base.yml -f docker-compose.override.yml

# ====================
# PRIMARY TARGETS
# ====================

.PHONY: help up down restart clean logs attach connect ssh status info

# Default target - show help
help:
	@echo "EdgeLake Docker Compose - Layered Configuration"
	@echo ""
	@echo "Usage:"
	@echo "  make <command> [deployment-type]"
	@echo ""
	@echo "Deployment Types:"
	@echo "  master, query (default: $(DEFAULT_EDGELAKE_TYPE))"
	@echo ""
	@echo "Commands:"
	@echo "  sync [type]        - Sync deployment to remote host"
	@echo "  pull [type]        - Pull image from registry and tag locally"
	@echo "  up [type]          - Start deployment"
	@echo "  down [type]        - Stop deployment"
	@echo "  restart [type]     - Restart deployment"
	@echo "  clean [type]       - Stop and remove volumes"
	@echo "  logs [type]        - View logs (follow mode)"
	@echo "  attach [type]      - Attach to EdgeLake CLI (Ctrl-D to detach)"
	@echo "  connect [type]     - Connect to container bash shell"
	@echo "  ssh [type]         - SSH to deployment host"
	@echo "  status [type]      - Show deployment status"
	@echo "  info [type]        - Show deployment configuration"
	@echo ""
	@echo "Batch Commands:"
	@echo "  sync-all           - Sync all deployments to their remote hosts"
	@echo "  up-all             - Start all deployments"
	@echo "  down-all           - Stop all deployments"
	@echo "  restart-all        - Restart all deployments"
	@echo "  clean-all          - Clean all deployments"
	@echo "  status-all         - Show status of all deployments"
	@echo ""
	@echo "Examples:"
	@echo "  make sync master                - Sync master to remote host"
	@echo "  make pull query                 - Pull query image from registry"
	@echo "  make up master                  - Start master deployment"
	@echo "  make attach query               - Attach to query node CLI"
	@echo "  make logs master                - View master logs"
	@echo "  make clean query                - Stop query and remove volumes"
	@echo "  make info master                - Show master configuration"
	@echo ""
	@echo "Quick Start:"
	@echo "  make sync-all                   - Sync all deployments to remote hosts"
	@echo "  make up-all                     - Start all nodes in order"
	@echo "  make status-all                 - Check all nodes"
	@echo "  make down-all                   - Stop all nodes"

# Start deployment
up:
	@echo "=== Starting $(EDGELAKE_TYPE) deployment ==="
	@echo "Node: $(NODE_NAME)"
	@echo "Image: $(IMAGE):$(TAG)"
	@$(COMPOSE) up -d
	@echo "✓ $(EDGELAKE_TYPE) started successfully"

# Stop deployment
down:
	@echo "=== Stopping $(EDGELAKE_TYPE) deployment ==="
	@echo "Node: $(NODE_NAME)"
	@$(COMPOSE) down
	@echo "✓ $(EDGELAKE_TYPE) stopped successfully"

# Restart deployment
restart:
	@echo "=== Restarting $(EDGELAKE_TYPE) deployment ==="
	@$(MAKE) down EDGELAKE_TYPE=$(EDGELAKE_TYPE)
	@echo "Waiting 5 seconds..."
	@sleep 5
	@$(MAKE) up EDGELAKE_TYPE=$(EDGELAKE_TYPE)

# Stop and clean volumes
clean:
	@echo "=== Cleaning $(EDGELAKE_TYPE) deployment ==="
	@echo "Node: $(NODE_NAME)"
	@echo "WARNING: This will remove all volumes and data!"
	@$(COMPOSE) down -v
	@echo "✓ $(EDGELAKE_TYPE) cleaned successfully"

# View logs
logs:
	@echo "=== Logs for $(EDGELAKE_TYPE) ($(NODE_NAME)) ==="
	@echo "Press Ctrl-C to exit"
	@$(COMPOSE) logs -f

# Attach to EdgeLake CLI
attach:
	@echo "=== Attaching to $(EDGELAKE_TYPE) EdgeLake CLI ==="
	@echo "Node: $(NODE_NAME)"
	@echo "Use Ctrl-D to detach"
	@$(DOCKER_SUDO) docker attach --detach-keys=ctrl-d $(NODE_NAME)

# Connect to container bash shell
connect:
	@echo "=== Connecting to $(EDGELAKE_TYPE) container ==="
	@echo "Node: $(NODE_NAME)"
	@$(DOCKER_SUDO) docker exec -it $(NODE_NAME) bash

# SSH to deployment host
ssh:
	@echo "=== SSH to $(EDGELAKE_TYPE) host ==="
	@echo "Host: $(SSH_HOST)"
ifeq ($(SSH_HOST),localhost)
	@echo "ERROR: OVERLAY_IP not set for $(EDGELAKE_TYPE), cannot SSH"
	@exit 1
else
	@ssh $(SSH_USER)@$(SSH_HOST)
endif

# Show deployment status
status:
	@echo "=== Status for $(EDGELAKE_TYPE) ==="
	@echo "Node: $(NODE_NAME)"
	@$(COMPOSE) ps
	@echo ""
	@echo "Container Details:"
	@$(DOCKER_SUDO) docker ps -a --filter "name=$(NODE_NAME)" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

# Show deployment configuration
info:
	@echo "=== Configuration for $(EDGELAKE_TYPE) deployment ==="
	@echo ""
	@echo "Deployment:"
	@echo "  Type:                      $(EDGELAKE_TYPE)"
	@echo "  Directory:                 $(DEPLOYMENT_DIR)"
	@echo ""
	@echo "Container:"
	@echo "  Node Name:                 $(NODE_NAME)"
	@echo "  Image:                     $(IMAGE):$(TAG)"
	@echo ""
	@echo "Network:"
	@echo "  Overlay IP:                $(or $(OVERLAY_IP),<not set>)"
	@echo "  AnyLog Server Port:        $(ANYLOG_SERVER_PORT)"
	@echo "  AnyLog REST Port:          $(ANYLOG_REST_PORT)"
	@echo ""
	@echo "SSH:"
	@echo "  SSH Host:                  $(SSH_HOST)"
	@echo "  SSH User:                  $(SSH_USER)"
	@echo ""
	@echo "Files:"
	@echo "  Base Config:               $(DEPLOYMENT_DIR)/configs/base_configs.env"
	@echo "  Advanced Config:           $(DEPLOYMENT_DIR)/configs/advance_configs.env"
	@echo "  Deployment Env:            $(DEPLOYMENT_DIR)/.env"
	@echo ""
	@echo "Volumes:"
	@$(DOCKER_SUDO) docker volume ls --filter "name=$(NODE_NAME)" --format "  {{.Name}}"

# ====================
# BATCH TARGETS (All Deployments)
# ====================

.PHONY: up-all down-all restart-all clean-all status-all

# Start all deployments in order (master first, query last)
up-all:
	@echo "=== Starting all deployments ==="
	@$(MAKE) up master
	@echo "Waiting 10 seconds for master to stabilize..."
	@sleep 10
	@$(MAKE) up query
	@echo "✓ All deployments started successfully"

# Stop all deployments in reverse order (query first, master last)
down-all:
	@echo "=== Stopping all deployments ==="
	@$(MAKE) down query
	@$(MAKE) down master
	@echo "✓ All deployments stopped successfully"

# Restart all deployments
restart-all:
	@echo "=== Restarting all deployments ==="
	@$(MAKE) down-all
	@echo "Waiting 10 seconds..."
	@sleep 10
	@$(MAKE) up-all

# Clean all deployments
clean-all:
	@echo "=== Cleaning all deployments ==="
	@echo "WARNING: This will remove ALL volumes and data!"
	@read -p "Are you sure? [y/N] " -n 1 -r; \
	echo; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		$(MAKE) clean query; \
		$(MAKE) clean master; \
		echo "✓ All deployments cleaned successfully"; \
	else \
		echo "Cancelled"; \
	fi

# Show status of all deployments
status-all:
	@echo "=== Status of all deployments ==="
	@echo ""
	@for type in $(DEPLOYMENT_TYPES); do \
		echo "--- $$type ---"; \
		$(MAKE) --no-print-directory status EDGELAKE_TYPE=$$type 2>/dev/null || echo "  Not running"; \
		echo ""; \
	done

# ====================
# UTILITY TARGETS
# ====================

.PHONY: pull rebuild exec sync sync-all

# Sync deployment to remote host
sync:
	@echo "=== Syncing $(EDGELAKE_TYPE) deployment to remote host ==="
	@echo "Host: $(SSH_HOST)"
	@echo "User: $(SSH_USER)"
	@echo "Path: $(REMOTE_DEPLOY_PATH)"
ifeq ($(SSH_HOST),localhost)
	@echo "ERROR: OVERLAY_IP not set for $(EDGELAKE_TYPE), cannot sync to remote"
	@exit 1
else
	@echo "Syncing trv-docker-compose directory..."
	@rsync -avz --delete \
		--exclude='.git' \
		--exclude='*.pyc' \
		--exclude='__pycache__' \
		--exclude='.env.local' \
		--exclude='.env.root' \
		../ $(SSH_USER)@$(SSH_HOST):$(REMOTE_DEPLOY_PATH)/
	@echo "✓ Sync complete to $(SSH_USER)@$(SSH_HOST):$(REMOTE_DEPLOY_PATH)/"
endif

# Sync all deployments to their respective hosts
sync-all:
	@echo "=== Syncing all deployments to remote hosts ==="
	@for type in $(DEPLOYMENT_TYPES); do \
		echo ""; \
		echo "--- Syncing $$type ---"; \
		$(MAKE) --no-print-directory sync EDGELAKE_TYPE=$$type || echo "  Skipped (localhost or error)"; \
	done
	@echo ""
	@echo "✓ All syncs complete"

# Pull latest image for deployment from registry
pull:
	@echo "=== Pulling image for $(EDGELAKE_TYPE) from registry ==="
	@echo "Registry: $(REGISTRY)"
	@echo "Image: $(IMAGE):$(TAG)"
	@echo "Pulling $(REGISTRY)/$(IMAGE):$(TAG)..."
	@$(DOCKER_SUDO) docker pull $(REGISTRY)/$(IMAGE):$(TAG)
	@echo "Tagging as $(IMAGE):$(TAG)..."
	@$(DOCKER_SUDO) docker tag $(REGISTRY)/$(IMAGE):$(TAG) $(IMAGE):$(TAG)
	@echo "✓ Image pulled and tagged successfully"

# Pull and restart deployment
rebuild:
	@echo "=== Rebuilding $(EDGELAKE_TYPE) deployment ==="
	@$(MAKE) pull EDGELAKE_TYPE=$(EDGELAKE_TYPE)
	@$(MAKE) down EDGELAKE_TYPE=$(EDGELAKE_TYPE)
	@$(MAKE) up EDGELAKE_TYPE=$(EDGELAKE_TYPE)

# Execute arbitrary command in container
# Usage: make exec operator cmd="ls -la"
exec:
	@echo "=== Executing in $(NODE_NAME) ==="
ifdef cmd
	@$(DOCKER_SUDO) docker exec -it $(NODE_NAME) $(cmd)
else
	@echo "ERROR: cmd parameter required"
	@echo "Usage: make exec operator cmd=\"ls -la\""
	@exit 1
endif

# ====================
# DISCOVERY TARGETS
# ====================

.PHONY: list-deployments list-volumes

# List all available deployments
list-deployments:
	@echo "=== Available Deployments ==="
	@ls -1 deployments/

# List all volumes for a deployment
list-volumes:
	@echo "=== Volumes for $(EDGELAKE_TYPE) ($(NODE_NAME)) ==="
	@$(DOCKER_SUDO) docker volume ls --filter "name=$(NODE_NAME)" --format "{{.Name}}\t{{.Driver}}\t{{.Mountpoint}}"

# ====================
# DUMMY TARGETS
# ====================
# Dummy targets for deployment types so they can be used as arguments
# Example: make attach query, make up operator2

.PHONY: $(DEPLOYMENT_TYPES)

$(DEPLOYMENT_TYPES):
	@:
