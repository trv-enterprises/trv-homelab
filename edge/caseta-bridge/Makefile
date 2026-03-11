# Caseta MQTT Bridge
# Makefile for deploying and managing the bridge on services LXC

DEPLOY_HOST ?= <services-tailscale-ip>
DEPLOY_USER ?= <user>
REMOTE_DIR := /opt/caseta-bridge
REMOTE_CONFIG := /etc/caseta-bridge/config.yaml
REMOTE_CERTS := /etc/caseta-bridge/certs
SERVICE := caseta-bridge.service

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

##@ Deploy

.PHONY: deploy
deploy: ## Full deploy: sync files, install deps, configure systemd, start
	scp bridge.py requirements.txt $(DEPLOY_USER)@$(DEPLOY_HOST):/tmp/
	scp caseta-bridge.service $(DEPLOY_USER)@$(DEPLOY_HOST):/tmp/$(SERVICE)
	@# Only deploy config if remote config doesn't exist yet
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) 'test -f $(REMOTE_CONFIG) || echo "NEED_CONFIG"' | \
		grep -q NEED_CONFIG && \
		scp config.yaml $(DEPLOY_USER)@$(DEPLOY_HOST):/tmp/config.yaml || true
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) '\
		sudo mkdir -p $(REMOTE_DIR) $(REMOTE_CERTS) /etc/caseta-bridge && \
		sudo mv /tmp/bridge.py $(REMOTE_DIR)/bridge.py && \
		sudo mv /tmp/requirements.txt $(REMOTE_DIR)/requirements.txt && \
		test -f /tmp/config.yaml && sudo mv /tmp/config.yaml $(REMOTE_CONFIG) || true && \
		sudo python3 -m venv $(REMOTE_DIR)/venv && \
		sudo $(REMOTE_DIR)/venv/bin/pip install -q -r $(REMOTE_DIR)/requirements.txt && \
		sudo mv /tmp/$(SERVICE) /etc/systemd/system/$(SERVICE) && \
		sudo systemctl daemon-reload && \
		sudo systemctl enable $(SERVICE) && \
		sudo systemctl restart $(SERVICE)'
	@echo "Deployed and started $(SERVICE)"

.PHONY: deploy-config
deploy-config: ## Deploy config only and reload (SIGHUP)
	scp config.yaml $(DEPLOY_USER)@$(DEPLOY_HOST):/tmp/config.yaml
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) '\
		sudo mv /tmp/config.yaml $(REMOTE_CONFIG) && \
		sudo systemctl reload $(SERVICE)'
	@echo "Config deployed and reloaded"

.PHONY: deploy-code
deploy-code: ## Deploy bridge.py only and restart
	scp bridge.py $(DEPLOY_USER)@$(DEPLOY_HOST):/tmp/bridge.py
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) '\
		sudo mv /tmp/bridge.py $(REMOTE_DIR)/bridge.py && \
		sudo systemctl restart $(SERVICE)'
	@echo "Code deployed and restarted"

##@ Pairing

.PHONY: pair
pair: ## Run pairing on remote host (BRIDGE_IP=<ip>)
	$(if $(BRIDGE_IP),,$(error BRIDGE_IP is required — make pair BRIDGE_IP=192.168.1.XXX))
	scp pair.sh $(DEPLOY_USER)@$(DEPLOY_HOST):/tmp/pair.sh
	ssh -t $(DEPLOY_USER)@$(DEPLOY_HOST) '\
		chmod +x /tmp/pair.sh && \
		sudo mkdir -p $(REMOTE_DIR) && \
		test -d $(REMOTE_DIR)/venv || sudo python3 -m venv $(REMOTE_DIR)/venv && \
		sudo $(REMOTE_DIR)/venv/bin/pip install -q "pylutron-caseta[cli]>=0.27.0" && \
		sudo PATH=$(REMOTE_DIR)/venv/bin:$$PATH /tmp/pair.sh $(BRIDGE_IP)'

##@ Remote Operations

.PHONY: logs
logs: ## View remote logs (follow mode)
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) 'sudo journalctl -u $(SERVICE) -f'

.PHONY: status
status: ## Show remote service status
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) 'sudo systemctl status $(SERVICE)'

.PHONY: restart
restart: ## Restart remote service
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) 'sudo systemctl restart $(SERVICE)'

.PHONY: stop
stop: ## Stop remote service
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) 'sudo systemctl stop $(SERVICE)'

.PHONY: devices
devices: ## Show discovered devices from bridge logs
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) 'sudo journalctl -u $(SERVICE) --no-pager | grep "device_id\|discovered\| -> "'
