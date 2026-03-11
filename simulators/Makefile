# Simulator Deployment Makefile
# Deploys simulators to LXD container on trv-srv-001

INVENTORY := inventory.yml
PLAYBOOK := deploy.yml
HOST := trv-srv-001
LXC := simulators
REMOTE_PATH := /root/simulators

.PHONY: help deploy sync build restart status logs shell clean

help:
	@echo "Simulator Deployment Commands:"
	@echo "  make deploy    - Full deployment (sync, build, start)"
	@echo "  make sync      - Sync source files only"
	@echo "  make build     - Rebuild Docker images"
	@echo "  make restart   - Restart all services"
	@echo "  make status    - Show service status"
	@echo "  make logs      - View logs (follow mode)"
	@echo "  make shell     - Open shell in LXD container"
	@echo "  make clean     - Stop and remove containers/volumes"

deploy:
	ansible-playbook -i $(INVENTORY) $(PLAYBOOK)

sync:
	ansible-playbook -i $(INVENTORY) $(PLAYBOOK) --tags sync

build:
	ssh <user>@<hub-tailscale-ip> "lxc exec $(LXC) -- bash -c 'cd $(REMOTE_PATH) && docker compose build'"

restart:
	ssh <user>@<hub-tailscale-ip> "lxc exec $(LXC) -- bash -c 'cd $(REMOTE_PATH) && docker compose restart'"

status:
	@ssh <user>@<hub-tailscale-ip> "lxc exec $(LXC) -- docker compose -f $(REMOTE_PATH)/docker-compose.yml ps"

logs:
	ssh <user>@<hub-tailscale-ip> "lxc exec $(LXC) -- docker compose -f $(REMOTE_PATH)/docker-compose.yml logs -f"

logs-%:
	ssh <user>@<hub-tailscale-ip> "lxc exec $(LXC) -- docker compose -f $(REMOTE_PATH)/docker-compose.yml logs -f $*"

shell:
	ssh -t <user>@<hub-tailscale-ip> "lxc exec $(LXC) -- bash"

clean:
	@echo "WARNING: This will stop all containers and remove volumes!"
	@read -p "Are you sure? [y/N] " confirm && [ "$$confirm" = "y" ]
	ssh <user>@<hub-tailscale-ip> "lxc exec $(LXC) -- bash -c 'cd $(REMOTE_PATH) && docker compose down -v'"

# Quick commands for individual services
up-%:
	ssh <user>@<hub-tailscale-ip> "lxc exec $(LXC) -- bash -c 'cd $(REMOTE_PATH) && docker compose up -d $*'"

down-%:
	ssh <user>@<hub-tailscale-ip> "lxc exec $(LXC) -- bash -c 'cd $(REMOTE_PATH) && docker compose stop $*'"

rebuild-%:
	ssh <user>@<hub-tailscale-ip> "lxc exec $(LXC) -- bash -c 'cd $(REMOTE_PATH) && docker compose build --no-cache $* && docker compose up -d $*'"
