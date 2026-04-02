# Weather Poller — Services LXC (101)
# Polls Visual Crossing API and publishes to MQTT

DEPLOY_HOST ?= <services-tailscale-ip>
DEPLOY_USER ?= <user>
REMOTE_DIR := /opt/services
SERVICE := weather-poller
ANSIBLE_DIR := ../../tools/ansible

GHCR_REGISTRY := ghcr.io/trv-enterprises
IMAGE := $(GHCR_REGISTRY)/weather-poller:latest

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

##@ Build & Push

.PHONY: build
build: ## Build Docker image for linux/amd64
	docker build --platform linux/amd64 -t $(IMAGE) .

.PHONY: push
push: ## Push image to GHCR
	@echo "$${TOM_GH_TOKEN}" | docker login ghcr.io -u trv-enterprises --password-stdin
	docker push $(IMAGE)

.PHONY: build-push
build-push: build push ## Build and push to GHCR

##@ Deploy

.PHONY: deploy
deploy: build-push ## Build, push to GHCR, deploy via Ansible
	cd $(ANSIBLE_DIR) && ansible-playbook playbooks/weather-poller-deploy.yml

##@ Remote Operations

.PHONY: logs
logs: ## View weather-poller logs (follow mode)
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) 'cd $(REMOTE_DIR) && sudo docker compose logs -f $(SERVICE)'

.PHONY: status
status: ## Show docker compose service status
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) 'cd $(REMOTE_DIR) && sudo docker compose ps $(SERVICE)'

.PHONY: restart
restart: ## Restart weather-poller container
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) 'cd $(REMOTE_DIR) && sudo docker compose restart $(SERVICE)'

.PHONY: stop
stop: ## Stop weather-poller container
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) 'cd $(REMOTE_DIR) && sudo docker compose stop $(SERVICE)'

##@ Testing

.PHONY: test-api
test-api: ## Test the Visual Crossing API with current .env
	@source .env 2>/dev/null && curl -s "https://weather.visualcrossing.com/VisualCrossingWebServices/rest/services/timeline/$${WEATHER_LOCATION}?unitGroup=us&include=current,alerts&key=$${VISUAL_CROSSING_KEY}&contentType=json" | python3 -m json.tool | head -30

.PHONY: test-mqtt
test-mqtt: ## Subscribe to weather MQTT topics (requires mosquitto_sub)
	mosquitto_sub -h $(DEPLOY_HOST) -t 'weather/#' -v
