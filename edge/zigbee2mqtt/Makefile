# Zigbee2MQTT on Services LXC (101)
# Manages Zigbee2MQTT alongside Mosquitto via Docker Compose

DEPLOY_HOST ?= <services-tailscale-ip>
DEPLOY_USER ?= <user>
REMOTE_DIR := /opt/services
COMPOSE_FILE := docker-compose.yml
Z2M_SERVICE := zigbee2mqtt
Z2M_API := http://$(DEPLOY_HOST):8080/api

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

##@ Deploy

.PHONY: deploy
deploy: ## Sync compose file + config, restart full stack
	scp docker-compose.services.yml $(DEPLOY_USER)@$(DEPLOY_HOST):/tmp/$(COMPOSE_FILE)
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) '\
		sudo mv /tmp/$(COMPOSE_FILE) $(REMOTE_DIR)/$(COMPOSE_FILE) && \
		cd $(REMOTE_DIR) && \
		sudo docker compose up -d'
	@echo "Waiting for zigbee2mqtt to create initial config..."
	@sleep 5
	$(MAKE) deploy-config
	@echo "Stack deployed"

.PHONY: deploy-config
deploy-config: ## Sync Z2M config only, restart zigbee2mqtt container (WARNING: overwrites network_key)
	@echo "WARNING: This will overwrite the live configuration including the network key."
	@echo "If devices are already paired, they will need to be re-paired."
	@read -p "Continue? [y/N] " confirm && [ "$$confirm" = "y" ] || exit 1
	@echo "Copying configuration.yaml into zigbee2mqtt_data volume..."
	scp configuration.yaml $(DEPLOY_USER)@$(DEPLOY_HOST):/tmp/z2m-configuration.yaml
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) '\
		VOLUME_PATH=$$(sudo docker volume inspect services_zigbee2mqtt_data -f "{{.Mountpoint}}") && \
		sudo cp /tmp/z2m-configuration.yaml $$VOLUME_PATH/configuration.yaml && \
		rm /tmp/z2m-configuration.yaml && \
		cd $(REMOTE_DIR) && \
		sudo docker compose restart $(Z2M_SERVICE)'
	@echo "Config deployed and zigbee2mqtt restarted"

##@ Remote Operations

.PHONY: logs
logs: ## View zigbee2mqtt logs (follow mode)
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) 'cd $(REMOTE_DIR) && sudo docker compose logs -f $(Z2M_SERVICE)'

.PHONY: status
status: ## Show docker compose service status
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) 'cd $(REMOTE_DIR) && sudo docker compose ps'

.PHONY: restart
restart: ## Restart zigbee2mqtt container
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) 'cd $(REMOTE_DIR) && sudo docker compose restart $(Z2M_SERVICE)'

.PHONY: stop
stop: ## Stop zigbee2mqtt container
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) 'cd $(REMOTE_DIR) && sudo docker compose stop $(Z2M_SERVICE)'

##@ Pairing

.PHONY: permit-join
permit-join: ## Enable pairing mode via Z2M API
	curl -s -X POST $(Z2M_API)/permit_join -H 'Content-Type: application/json' \
		-d '{"value": true}' | python3 -m json.tool
	@echo "Pairing mode ENABLED — devices can join"

.PHONY: stop-join
stop-join: ## Disable pairing mode via Z2M API
	curl -s -X POST $(Z2M_API)/permit_join -H 'Content-Type: application/json' \
		-d '{"value": false}' | python3 -m json.tool
	@echo "Pairing mode DISABLED"

##@ USB Passthrough (run once on Proxmox host)

.PHONY: setup-usb
setup-usb: ## Show commands to run on Proxmox host for USB passthrough
	@echo "Run these commands on trv-srv-002 as root:"
	@echo ""
	@echo "  # Add USB passthrough to LXC 101 config"
	@echo '  cat >> /etc/pve/lxc/101.conf << EOF'
	@echo "  lxc.cgroup2.devices.allow: c 188:* rwm"
	@echo "  lxc.mount.entry: /dev/ttyUSB0 dev/ttyUSB0 none bind,optional,create=file"
	@echo "  EOF"
	@echo ""
	@echo "  # Reboot LXC 101"
	@echo "  pct reboot 101"
	@echo ""
	@echo "  # Verify USB is visible inside container"
	@echo "  pct exec 101 -- ls -la /dev/ttyUSB0"
