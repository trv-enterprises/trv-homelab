# Ansible Deployments & Server Reports

Ansible playbooks and roles for deploying homelab services and generating infrastructure reports.

## Quick Start

```bash
# Install Ansible (macOS)
brew install ansible

# Copy inventory template and fill in your IPs
cp inventory/hosts.yml.example inventory/hosts.yml

# Create vault with your secrets
cp inventory/group_vars/all/vault.yml.example inventory/group_vars/all/vault.yml
ansible-vault encrypt inventory/group_vars/all/vault.yml

# Test connectivity
cd tools/ansible
ansible all -m ping

# Deploy a service
ansible-playbook playbooks/dashboard-deploy.yml
ansible-playbook playbooks/weather-poller-deploy.yml

# Generate server reports
ansible-playbook playbooks/server-report.yml
```

## Playbooks

| Playbook | Purpose |
|----------|---------|
| `dashboard-deploy.yml` | Deploy dashboard app (Docker Compose) |
| `weather-poller-deploy.yml` | Deploy weather poller (Docker, GHCR) |
| `tsstore-deploy.yml` | Deploy ts-store binary + collectors |
| `our-kiosk-deploy.yml` | Deploy kiosk voice + display |
| `our-kiosk-setup.yml` | Initial kiosk setup (Node.js) |
| `our-kiosk-setup-minisforum.yml` | Minisforum kiosk setup (X11, Node, Python) |
| `alert-engine-deploy.yml` | Deploy alert engine (Docker, GHCR) |
| `server-report.yml` | Generate infrastructure reports |

## Directory Structure

```
tools/ansible/
├── ansible.cfg                          # Ansible configuration
├── inventory/
│   ├── hosts.yml.example                # Inventory template (fill in your IPs)
│   └── group_vars/all/
│       └── vault.yml.example            # Vault template (API keys, tokens)
├── playbooks/                           # Deployment and reporting playbooks
├── roles/
│   ├── services-stack/                  # Shared docker-compose for services LXC
│   ├── alert-engine/                    # Alert engine deployment
│   ├── dashboard/                       # Dashboard app deployment
│   ├── weather-poller/                  # Weather poller deployment
│   ├── tsstore/                         # ts-store binary deployment
│   ├── voice-display/                   # Kiosk voice pipeline deployment
│   └── server-report/                   # Infrastructure reporting
└── reports/                             # Generated reports (gitignored)
```

## Two-Repo Workflow (Recommended)

This repository contains all Ansible roles, playbooks, and example configs with placeholder
values. Real deployment requires a **private deploy repo** with your actual inventory,
secrets, and deployment-specific configuration files.

### What goes where

| Content | Repository |
|---------|-----------|
| Ansible roles, playbooks, templates | trv-homelab (public) |
| Example configs with placeholder values | trv-homelab (public) |
| Real IP addresses, hostnames | your deploy repo (private) |
| Vault secrets (API keys, tokens) | your deploy repo (private) |
| Deployment-specific config files (alert rules, etc.) | your deploy repo (private) |

### Setup

1. **Clone trv-homelab** (don't fork -- pull upstream updates directly):
   ```bash
   git clone https://github.com/trv-enterprises/trv-homelab.git
   ```

2. **Create your deploy repo** with this structure:
   ```
   my-homelab-deploy/
   ├── Makefile                    # Targets wrapping ansible-playbook
   ├── ansible.cfg                 # Points to your inventory + vault
   ├── inventory/
   │   ├── hosts.yml               # Real IPs for your hosts
   │   └── group_vars/all/
   │       └── vault.yml           # Encrypted secrets (GHCR token, API keys)
   ├── host_vars/                  # Per-host variable overrides
   └── files/                      # Deployment-specific config files
       └── alert-engine/
           └── rules.yaml          # Your actual alert rules
   ```

3. **Copy and fill in templates**:
   ```bash
   # From trv-homelab/tools/ansible/
   cp inventory/hosts.yml.example ~/my-homelab-deploy/inventory/hosts.yml
   cp inventory/group_vars/all/vault.yml.example ~/my-homelab-deploy/inventory/group_vars/all/vault.yml
   ansible-vault encrypt ~/my-homelab-deploy/inventory/group_vars/all/vault.yml

   # Copy example alert rules and customize with your device names
   cp ../../edge/sensor-alert-engine/rules.yaml ~/my-homelab-deploy/files/alert-engine/rules.yaml
   ```

4. **Run playbooks** from your deploy repo using `-i` to point at your inventory:
   ```bash
   ansible-playbook -i inventory \
     ../trv-homelab/tools/ansible/playbooks/weather-poller-deploy.yml
   ```

### Example Makefile

```makefile
PLAYBOOK_DIR = ../trv-homelab/tools/ansible/playbooks
INVENTORY = inventory

deploy-weather:
    ansible-playbook -i $(INVENTORY) $(PLAYBOOK_DIR)/weather-poller-deploy.yml

deploy-alert-engine:
    ansible-playbook -i $(INVENTORY) $(PLAYBOOK_DIR)/alert-engine-deploy.yml \
        -e "alert_engine_rules_file=$(CURDIR)/files/alert-engine/rules.yaml"

deploy-dashboard:
    ansible-playbook -i $(INVENTORY) $(PLAYBOOK_DIR)/dashboard-deploy.yml
```

## Adding New Applications to Reports

Edit `roles/server-report/vars/apps.yml`:

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

## Troubleshooting

**Host unreachable:**
- Check Tailscale is connected: `tailscale status`
- Test SSH directly: `ssh <user>@<ip>`

**Python version issues:**
- Ansible 13+ requires Python 3.10+ on targets
- Older devices may need Ansible 9.x or use raw module
