# Simulator Deployment

Ansible-based deployment for the dashboard data source simulators to an LXD container.

## Architecture

```
trv-srv-001 (<hub-tailscale-ip>)
└── LXD Container: simulators (<simulator-tailscale-ip>)
    └── Docker Compose Stack
        ├── ts-store (port 21080)
        ├── websocket (port 21081)
        ├── rest-api (port 21082)
        ├── csv-server (port 21083)
        ├── postgres (port 21432)
        ├── data-writer
        └── db-seeder
```

## Prerequisites

- Ansible installed locally
- SSH access to trv-srv-001 (<hub-tailscale-ip>)
- LXD container "simulators" already created with Docker installed

## Files

- `inventory.yml` - Ansible inventory with host definitions
- `deploy.yml` - Main deployment playbook
- `env/production.env` - Production environment variables (secrets)
- `env/production.env.example` - Template for production env

## Usage

```bash
# Deploy/update simulators
make deploy

# View status
make status

# View logs
make logs

# Restart services
make restart

# Full redeploy (rebuild images)
make redeploy
```

## Manual LXD Commands

If needed, access the container directly:

```bash
# SSH to host
ssh <user>@<hub-tailscale-ip>

# Access container
lxc exec simulators -- bash

# Docker commands inside container
cd /root/simulators
docker compose ps
docker compose logs -f
```

## Simulator Source

The simulator source code lives in `dashboard/simulators/`. This deployment
pulls that code and deploys it to the LXD container.

## Endpoints (via Tailscale)

| Service | Endpoint |
|---------|----------|
| ts-store | http://<simulator-tailscale-ip>:21080 |
| WebSocket | ws://<simulator-tailscale-ip>:21081/ws |
| REST API | http://<simulator-tailscale-ip>:21082 |
| CSV | http://<simulator-tailscale-ip>:21083 |
| PostgreSQL | <simulator-tailscale-ip>:21432 |
