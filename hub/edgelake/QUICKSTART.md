# Quick Start Guide

## TL;DR (Using Makefile - Recommended)

```bash
# Sync deployment to remote host
make sync operator

# Start deployment (on remote)
make up operator

# View logs
make logs operator

# SSH to remote host
make ssh operator
```

## TL;DR (Direct Docker Compose)

```bash
# Start a deployment
cd trv-docker-compose/deployments/master
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml up -d

# Stop a deployment
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml down

# Clean volumes
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml down -v
```

## Remote Deployment Workflow

```bash
# 1. Sync all deployments to their remote hosts
make sync-all

# 2. SSH to each host and start the deployment
make ssh operator
# On remote: cd /home/USER/EdgeLake/docker-compose/trv-docker-compose && make up operator
```

## All Deployments

### Master Node

```bash
cd trv-docker-compose/deployments/master
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml up -d
```

### Operator Node

```bash
cd trv-docker-compose/deployments/operator
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml up -d
```

### Operator2 Node

```bash
cd trv-docker-compose/deployments/operator2
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml up -d
```

### Query Node

```bash
cd trv-docker-compose/deployments/query
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml up -d
```

## Common Tasks

### View Logs

```bash
cd trv-docker-compose/deployments/master
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml logs -f
```

### Restart a Node

```bash
cd trv-docker-compose/deployments/master
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml restart
```

### Execute Commands in Container

```bash
cd trv-docker-compose/deployments/master
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml exec edgelake-master /bin/bash
```

### Pull Latest Image

```bash
cd trv-docker-compose/deployments/master
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml pull
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml up -d
```

## Configuration Overrides

### Quick Override in .env

Edit `deployments/<name>/.env`:

```bash
# Example: deployments/master/.env
NODE_NAME=my-master
OVERLAY_IP=<cam-porch-lan-ip>
TAG=dev
```

### One-Time Command-Line Override

```bash
cd trv-docker-compose/deployments/master
NODE_NAME=test-master TAG=dev docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml up -d
```

### Local Machine Override (gitignored)

Create `deployments/<name>/.env.local`:

```bash
# Example: deployments/master/.env.local
OVERLAY_IP=127.0.0.1
DEBUG_MODE=true
```

Uncomment the `.env.local` line in `docker-compose.base.yml`.

## Creating Helper Scripts

Make your life easier with wrapper scripts:

```bash
cd trv-docker-compose/deployments/master

# Create up.sh
cat > up.sh << 'EOF'
#!/bin/bash
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml up -d "$@"
EOF

# Create down.sh
cat > down.sh << 'EOF'
#!/bin/bash
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml down "$@"
EOF

# Create logs.sh
cat > logs.sh << 'EOF'
#!/bin/bash
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml logs -f "$@"
EOF

# Make executable
chmod +x up.sh down.sh logs.sh

# Now use simply:
./up.sh
./logs.sh
./down.sh
```

## Comparison with Old Method

### Old Method (docker-makefiles)

```bash
cd docker-makefiles
EDGELAKE_TYPE=master docker-compose -f docker-compose-template-base.yaml up
```

### New Method (trv-docker-compose)

```bash
cd trv-docker-compose/deployments/master
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml up -d
```

Or with helper script:

```bash
cd trv-docker-compose/deployments/master
./up.sh
```

## Benefits

✅ **Docker Native** - Standard Docker Compose override pattern
✅ **Independent** - Each deployment completely isolated
✅ **Flexible** - Multiple configuration layers
✅ **Clean** - No variable substitution or template generation
✅ **Scalable** - Easy to add new deployments

See [README.md](README.md) for full documentation.
