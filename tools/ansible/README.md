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
│   ├── dashboard/                       # Dashboard app deployment
│   ├── weather-poller/                  # Weather poller deployment
│   ├── tsstore/                         # ts-store binary deployment
│   ├── voice-display/                   # Kiosk voice pipeline deployment
│   └── server-report/                   # Infrastructure reporting
└── reports/                             # Generated reports (gitignored)
```

## Using with homelab-deploy

If you keep your real inventory in a separate private repo (recommended), point to it with `-i`:

```bash
ansible-playbook -i ~/homelab-deploy/inventory playbooks/dashboard-deploy.yml
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
