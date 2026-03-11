# Ansible Server Reports

Ansible-based infrastructure reporting for the homelab. Generates comprehensive server inventories with application detection and health checks.

## Quick Start

```bash
# Install Ansible (macOS)
brew install ansible

# Test connectivity
cd tools/ansible
ansible all -m ping

# Generate reports
ansible-playbook playbooks/server-report.yml

# Generate report for single host
ansible-playbook playbooks/server-report.yml --limit trv-srv-001
```

## Reports

Reports are generated in `reports/<timestamp>/` with:
- **Per-host reports** (`<hostname>.md`) - Detailed system info and app inventory
- **Fleet summary** (`00-INVENTORY.md`) - Combined view of all hosts
- **Latest symlink** (`reports/latest/`) - Points to most recent scan

### What's Collected

**System Resources:**
- CPU model, cores, load average
- Memory usage
- Disk usage by mount point
- Uptime

**Application Inventory:**
- EdgeLake nodes (master, query, operator)
- Open Horizon components (exchange, agbot, CSS, horizon agent)
- ts-store instances
- Data collectors (sensehat, system-stats, journal, shelly)
- Mosquitto MQTT
- Kubernetes (k3s)
- Docker Registry

**Health Checks:**
- EdgeLake REST endpoints
- Open Horizon Exchange API
- ts-store REST API
- Docker Registry API

**Infrastructure:**
- Docker containers and volumes
- Systemd services (system and user level)
- Listening TCP ports

## Inventory

Host inventory is defined in `inventory/hosts.yml`:

| Host | IP | Role |
|------|----|----|
| trv-srv-001 | <hub-tailscale-ip> | Hub (master, query, OH hub) |
| trv-srv-012 | <edge-srv-012-tailscale-ip> | EdgeLake operator (docker-compose) |
| trv-srv-014 | <edge-srv-014-tailscale-ip> | EdgeLake operator (Open Horizon) |
| trv-pi-001 | <pi-001-tailscale-ip> | ts-store + SenseHat |
| trv-pi-002 | <pi-002-tailscale-ip> | Mosquitto + ts-store + Shelly |
| trv-jetson-nano | <jetson-tailscale-ip> | Motion detector |

## Directory Structure

```
tools/ansible/
├── ansible.cfg              # Ansible configuration
├── inventory/
│   └── hosts.yml            # Host inventory
├── playbooks/
│   └── server-report.yml    # Main reporting playbook
├── roles/
│   └── server-report/
│       ├── tasks/           # Detection and collection tasks
│       ├── templates/       # Report templates (Jinja2)
│       └── vars/            # App definitions
└── reports/                 # Generated reports (gitignored)
```

## Adding New Applications

To detect new applications, edit `roles/server-report/vars/apps.yml`:

```yaml
app_definitions:
  my_app:
    display_name: My Application
    detection:
      docker_patterns:
        - "my-app-*"
      systemd_services:
        - my-app
      ports:
        - 8080
    health_checks:
      - name: api
        port: 8080
        path: /health
```

Then add detection logic in `roles/server-report/tasks/apps.yml` and update the templates.

## Migration to New Server

The Ansible setup is portable. To run from a different machine:

1. Install Ansible: `sudo apt install ansible` (Ubuntu) or `brew install ansible` (macOS)
2. Clone the repo
3. Set up SSH keys for <user>
4. Run: `cd tools/ansible && ansible-playbook playbooks/server-report.yml`

## Troubleshooting

**Host unreachable:**
- Check Tailscale is connected: `tailscale status`
- Test SSH directly: `ssh <user>@<ip>`

**Python version issues (Jetson):**
- Ansible 13+ requires Python 3.10+ on targets
- Older devices may need Ansible 9.x or use raw module

**Health check failures:**
- Some apps don't expose root path; adjust path in vars/apps.yml
- Check if service is actually responding: `curl http://<ip>:<port>/`
