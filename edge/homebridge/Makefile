# Homebridge MQTT → HomeKit bridge
# Deployed on services LXC (101) alongside Mosquitto, Z2M, weather-poller
#
# SCOPE: this Makefile deploys ONLY Homebridge's config.json and z2m-codec.js.
# The docker-compose.yml for the services stack is owned by the services-stack
# Ansible role (homelab-deploy: `make deploy-alert-engine`, which runs the
# role). Do NOT scp a compose file from this directory — an older copy lived
# in edge/zigbee2mqtt/ and would revert Mosquitto to a named-volume config
# (the 2026-04-11 persistence incident), drop the :9883 listener, and delete
# the alert-engine service outright.
#
# config.json here is an accessory-only FRAGMENT — just an `accessories`
# array, never `bridge` or `platforms`. Those are host state: the HomeKit
# pairing PIN, the bridge's MAC identity, and the Config UI platform on :8581.
# They stay on the host, so this public repo holds no secrets and no one can
# overwrite a live config with a half-populated copy.
#
# `make deploy-config` therefore MERGES accessories (by name) and the codec
# into the live config rather than replacing it, and writes a timestamped
# backup first. merge-config.py refuses to run if the repo file grows a
# bridge/platforms section, or if the live file lacks a bridge.pin.

# Sanitized per this repo's convention (real values live in homelab-deploy).
# Override on the command line or in the environment:
#   make deploy SERVICES_HOST=user@host
DEPLOY_HOST ?= <services-tailscale-ip>
DEPLOY_USER ?= <user>
SERVICES_HOST ?= $(DEPLOY_USER)@$(DEPLOY_HOST)
DEPLOY_DIR := /opt/services
VOLUME := services_homebridge_data

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

##@ Deploy

.PHONY: deploy
deploy: deploy-config restart ## Merge config + codec, then restart Homebridge

.PHONY: deploy-config
deploy-config: ## Merge accessories/codec into the LIVE config (backs up first)
	@command -v python3 >/dev/null || { echo "python3 required on the controller"; exit 1; }
	@python3 -c "import json; json.load(open('config.json'))" \
		|| { echo "config.json is not valid JSON"; exit 1; }
	@node --check z2m-codec.js 2>/dev/null \
		|| echo "note: node not available locally, skipping codec syntax check"
	scp -q config.json $(SERVICES_HOST):/tmp/hb-config-repo.json
	scp -q z2m-codec.js $(SERVICES_HOST):/tmp/z2m-codec.js
	scp -q merge-config.py $(SERVICES_HOST):/tmp/hb-merge-config.py
	ssh $(SERVICES_HOST) 'set -e; \
		VOL=$$(docker volume inspect $(VOLUME) -f "{{.Mountpoint}}"); \
		TS=$$(date +%Y%m%d-%H%M%S); \
		sudo cp $$VOL/config.json /tmp/hb-config-backup-$$TS.json; \
		echo "backup: /tmp/hb-config-backup-$$TS.json"; \
		sudo cat $$VOL/config.json > /tmp/hb-config-live.json; \
		python3 /tmp/hb-merge-config.py /tmp/hb-config-live.json /tmp/hb-config-repo.json /tmp/hb-config-merged.json; \
		sudo cp /tmp/hb-config-merged.json $$VOL/config.json; \
		sudo cp /tmp/z2m-codec.js $$VOL/z2m-codec.js; \
		sudo chown root:root $$VOL/config.json $$VOL/z2m-codec.js; \
		rm -f /tmp/hb-config-repo.json /tmp/hb-config-live.json /tmp/hb-config-merged.json /tmp/z2m-codec.js /tmp/hb-merge-config.py; \
		echo "config + codec merged into $$VOL"'

##@ Remote Operations

.PHONY: logs
logs: ## View remote logs (follow mode)
	ssh $(SERVICES_HOST) "cd $(DEPLOY_DIR) && docker compose logs -f homebridge"

.PHONY: status
status: ## Show remote service status
	ssh $(SERVICES_HOST) "cd $(DEPLOY_DIR) && docker compose ps homebridge"

.PHONY: accessories
accessories: ## List accessories in the LIVE config
	@ssh $(SERVICES_HOST) 'VOL=$$(docker volume inspect $(VOLUME) -f "{{.Mountpoint}}"); \
		sudo cat $$VOL/config.json' \
		| python3 -c "import json,sys; d=json.load(sys.stdin); [print(' -', a.get('name'), '('+a.get('type','?')+')') for a in d.get('accessories',[])]"

.PHONY: restart
restart: ## Restart Homebridge and show startup errors
	ssh $(SERVICES_HOST) "cd $(DEPLOY_DIR) && docker compose restart homebridge"
	@echo "waiting for Homebridge to initialize..."
	@sleep 20
	@ssh $(SERVICES_HOST) 'docker logs --since 60s services-homebridge-1 2>&1 \
		| grep -iE "error|invalid|NaN|failed" | tail -10' \
		|| echo "no startup errors"

.PHONY: stop
stop: ## Stop Homebridge
	ssh $(SERVICES_HOST) "cd $(DEPLOY_DIR) && docker compose stop homebridge"

.PHONY: backups
backups: ## List config backups on the services LXC
	@ssh $(SERVICES_HOST) 'ls -la /tmp/hb-config-backup-*.json 2>/dev/null || echo "(no backups)"'
