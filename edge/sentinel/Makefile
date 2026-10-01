# sentinel — Services LXC (101)
# Homelab alerts (Marshal + Frigate) -> SQLite -> APNs push + API for the trv-sentinel iOS app.
#
# Releases are built by CI from a `sentinel/vX.Y.Z` tag (see
# .github/workflows/publish-containers.yml) and deployed from homelab-deploy
# with `make deploy-sentinel`. The targets here are for local development.

SERVICE := sentinel
BROKER ?= <services-tailscale-ip>
TOKEN ?= $(API_TOKEN)
BASE ?= http://localhost:8070

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

##@ Build & Test

.PHONY: test
test: ## vet + race tests
	go vet ./... && go test -race ./...

.PHONY: build
build: ## Build the binary into ./bin
	mkdir -p bin && CGO_ENABLED=0 go build -o bin/$(SERVICE) .

.PHONY: run
run: ## Run locally against the live broker (needs API_TOKEN, FRIGATE_URL, MQTT_BROKER in env)
	go run .

.PHONY: docker-build
docker-build: ## Build the container image locally
	docker build -t ghcr.io/trv-enterprises/$(SERVICE):dev .

##@ Testing against a running server

.PHONY: pub-sensor
pub-sensor: ## Publish a fake Marshal "new" alert (TYPE=new|repeat|resolved RULE=test_rule)
	mosquitto_pub -h $(BROKER) -t sensors/alerts -m '{"type":"$(or $(TYPE),new)","source":"alert_engine","severity":"$(or $(SEV),warning)","rule":"$(or $(RULE),test_rule)","message":"Test alert from make","device":"Test Device","timestamp":"'"$$(date -u +%Y-%m-%dT%H:%M:%SZ)"'"}'

.PHONY: alerts
alerts: ## List alerts from the server
	curl -s -H "Authorization: Bearer $(TOKEN)" $(BASE)/v1/alerts | jq .

.PHONY: events
events: ## Follow the SSE stream
	curl -s -N -H "Authorization: Bearer $(TOKEN)" $(BASE)/v1/events

.PHONY: push-test
push-test: ## Send a test push to every registered device
	curl -s -X POST -H "Authorization: Bearer $(TOKEN)" $(BASE)/v1/push/test | jq .
