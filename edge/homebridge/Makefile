# Homebridge MQTT → HomeKit bridge
# Deployed on services LXC (101) alongside Mosquitto, Z2M, weather-poller

SERVICES_HOST := <user>@<services-tailscale-ip>
DEPLOY_DIR := /opt/services

.PHONY: deploy deploy-config logs status restart stop

# Full deploy: sync compose + config, restart stack
deploy: deploy-config
	ssh $(SERVICES_HOST) "cd $(DEPLOY_DIR) && docker compose pull homebridge && docker compose up -d homebridge"
	@echo "Homebridge deployed. Install mqttthing plugin via UI at http://<services-tailscale-ip>:8581"

# Sync config and codec files to the homebridge volume
deploy-config:
	ssh $(SERVICES_HOST) "docker compose -f $(DEPLOY_DIR)/docker-compose.yml up -d homebridge 2>/dev/null || true"
	@# Find the volume mount path and copy config + codec
	ssh $(SERVICES_HOST) '\
		VOL=$$(docker volume inspect services_homebridge_data -f "{{.Mountpoint}}"); \
		cp $(DEPLOY_DIR)/homebridge-config.json $$VOL/config.json; \
		cp $(DEPLOY_DIR)/z2m-codec.js $$VOL/z2m-codec.js; \
		echo "Config and codec copied to $$VOL"'

# Sync files to services LXC
sync:
	scp config.json $(SERVICES_HOST):$(DEPLOY_DIR)/homebridge-config.json
	scp z2m-codec.js $(SERVICES_HOST):$(DEPLOY_DIR)/z2m-codec.js
	scp ../zigbee2mqtt/docker-compose.services.yml $(SERVICES_HOST):$(DEPLOY_DIR)/docker-compose.yml

logs:
	ssh $(SERVICES_HOST) "cd $(DEPLOY_DIR) && docker compose logs -f homebridge"

status:
	ssh $(SERVICES_HOST) "cd $(DEPLOY_DIR) && docker compose ps homebridge"

restart:
	ssh $(SERVICES_HOST) "cd $(DEPLOY_DIR) && docker compose restart homebridge"

stop:
	ssh $(SERVICES_HOST) "cd $(DEPLOY_DIR) && docker compose stop homebridge"
