# EdgeLake Docker Compose - Layered Configuration

This directory contains a modern, layered Docker Compose setup for EdgeLake nodes using the Docker Compose override pattern. Each deployment is completely independent with its own configuration and environment.

## Directory Structure

```
trv-docker-compose/
├── docker-compose.base.yml          # Shared base configuration (DO NOT EDIT for deployments)
├── deployments/
│   ├── master/                      # Master node deployment
│   │   ├── docker-compose.override.yml
│   │   ├── .env                     # Deployment-specific variables
│   │   └── configs/
│   │       ├── base_configs.env     # Base node configuration
│   │       └── advance_configs.env  # Advanced settings
│   ├── operator/                    # Operator node deployment
│   │   ├── docker-compose.override.yml
│   │   ├── .env
│   │   └── configs/
│   │       ├── base_configs.env
│   │       └── advance_configs.env
│   ├── operator2/                   # Second operator deployment
│   │   ├── docker-compose.override.yml
│   │   ├── .env
│   │   └── configs/
│   │       ├── base_configs.env
│   │       └── advance_configs.env
│   └── query/                       # Query node deployment
│       ├── docker-compose.override.yml
│       ├── .env
│       └── configs/
│           ├── base_configs.env
│           └── advance_configs.env
└── README.md
```

## Configuration Priority

Environment variables are loaded in the following priority (highest to lowest):

1. **Command-line variables** - Passed directly when running docker-compose
2. **Shell environment variables** - Set in your current shell session
3. **`.env` file** - Deployment-specific overrides (in deployment directory)
4. **`configs/advance_configs.env`** - Advanced settings
5. **`configs/base_configs.env`** - Base node configuration

## Quick Start with Makefile (Recommended)

The easiest way to manage deployments is using the Makefile:

```bash
# Sync deployment to remote host (uses OVERLAY_IP from configs)
make sync operator

# Sync all deployments to their respective remote hosts
make sync-all

# Start a deployment (on remote host after sync)
make up master

# View logs
make logs operator

# Attach to EdgeLake CLI
make attach query

# Stop a deployment
make down operator2

# Clean volumes
make clean master

# Show deployment info
make info query

# Start all deployments
make up-all

# SSH to deployment host
make ssh operator
```

Run `make help` for complete command reference.

### Remote Deployment Workflow

The typical workflow for managing remote deployments:

```bash
# 1. Sync deployment to remote host
make sync operator

# 2. SSH to remote host
make ssh operator

# 3. On the remote host, navigate to the deployment and start it
cd /home/USER/EdgeLake/docker-compose/trv-docker-compose
make up operator

# Or do it in one go from local machine:
make sync operator && ssh USER@HOST "cd /home/USER/EdgeLake/docker-compose/trv-docker-compose && make up operator"
```

## Usage (Direct Docker Compose)

### Starting a Deployment

Navigate to the deployment directory and run:

```bash
cd deployments/master
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml up -d
```

Or use the shorthand:

```bash
cd deployments/master
docker-compose up -d
```

**Note:** The second method only works if you create a local `docker-compose.yml` symlink or wrapper.

### Stopping a Deployment

```bash
cd deployments/master
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml down
```

### Viewing Logs

```bash
cd deployments/master
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml logs -f
```

### Cleaning Volumes

```bash
cd deployments/master
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml down -v
```

## Configuration Management

### Basic Configuration

Edit the files in each deployment's `configs/` directory:

- **`configs/base_configs.env`** - Core node settings (NODE_TYPE, ports, database, blockchain)
- **`configs/advance_configs.env`** - Advanced settings (networking, Nebula, monitoring)

### Deployment-Specific Overrides

Edit the `.env` file in the deployment directory to override any setting:

```bash
# deployments/master/.env
NODE_NAME=my-custom-master
OVERLAY_IP=<cam-porch-lan-ip>
COMPANY_NAME="My Company"
```

### Command-Line Overrides

Override any variable at runtime:

```bash
cd deployments/master
NODE_NAME=temp-master TAG=dev docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml up
```

### Local Development Overrides

Create a `.env.local` file (gitignored) for machine-specific settings:

```bash
# deployments/master/.env.local
OVERLAY_IP=127.0.0.1
DEBUG_MODE=true
```

Then uncomment the `.env.local` line in `docker-compose.base.yml`:

```yaml
env_file:
  - ./configs/base_configs.env
  - ./configs/advance_configs.env
  - ./.env
  - ./.env.local  # Uncomment this line
```

## Helper Scripts (Optional)

You can create helper scripts in each deployment directory for convenience:

### `deployments/master/up.sh`

```bash
#!/bin/bash
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml up -d
```

### `deployments/master/down.sh`

```bash
#!/bin/bash
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml down
```

### `deployments/master/clean.sh`

```bash
#!/bin/bash
docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml down -v
```

Make them executable:

```bash
chmod +x up.sh down.sh clean.sh
```

Then use:

```bash
./up.sh
./down.sh
./clean.sh
```

## Migration from Old Structure

If you're migrating from the `docker-makefiles/` structure:

1. **Configurations are already copied** - Each deployment's `configs/` directory contains the settings from the corresponding `docker-makefiles/*-configs/` directory

2. **Update your workflows** - Instead of using `EDGELAKE_TYPE` variable, navigate to the specific deployment directory

3. **Old way:**
   ```bash
   cd docker-makefiles
   EDGELAKE_TYPE=master docker-compose up
   ```

4. **New way:**
   ```bash
   cd trv-docker-compose/deployments/master
   docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml up
   ```

## Benefits of This Approach

1. **Docker Native** - Uses the official Docker Compose override pattern
2. **Independent Deployments** - Each deployment is self-contained and isolated
3. **Flexible Configuration** - Multiple layers of configuration with clear priority
4. **Easy to Version Control** - Separate deployment-specific from shared configuration
5. **Scalable** - Easy to add new deployments (just copy a deployment directory)
6. **No Template Substitution** - No need for custom scripts to generate compose files

## Duplicating to a New Environment

To use this setup in a different environment (e.g., production, staging):

1. **Copy the entire directory:**
   ```bash
   cp -r trv-docker-compose ~/production-edgelake
   cd ~/production-edgelake
   ```

2. **Create root-level configuration:**
   ```bash
   cp .env.root.template .env.root
   # Edit .env.root to set:
   #   - DEFAULT_IMAGE and DEFAULT_TAG
   #   - SSH_USER for remote access
   #   - Any other environment-wide defaults
   ```

3. **Update each deployment's configuration:**
   - Edit `deployments/*/configs/base_configs.env` - Ports, database, blockchain settings
   - Edit `deployments/*/configs/advance_configs.env` - OVERLAY_IP, networking, features
   - Edit `deployments/*/.env` - Image tags, node names (if needed)

4. **Use the Makefile:**
   ```bash
   make up-all      # Start all nodes
   make status-all  # Check status
   ```

The Makefile automatically discovers NODE_NAME, OVERLAY_IP, and ports from your configs!

## Adding a New Deployment

1. Copy an existing deployment directory:
   ```bash
   cp -r deployments/operator deployments/operator3
   ```

2. Edit the configuration files:
   ```bash
   cd deployments/operator3
   # Edit .env to set NODE_NAME, IMAGE, TAG, etc.
   # Edit configs/base_configs.env for node-specific settings
   # Edit configs/advance_configs.env for advanced settings
   ```

3. Start the deployment:
   ```bash
   make up operator3
   # or
   docker-compose -f ../../docker-compose.base.yml -f docker-compose.override.yml up -d
   ```

## Troubleshooting

### Variables Not Resolving

Make sure you're running docker-compose from within the deployment directory. The paths in `env_file` are relative to the current working directory.

### Port Conflicts

Check the ports in `configs/base_configs.env`:
- Master: 32048/32049
- Operator: 32148/32149
- Operator2: Different ports needed
- Query: 32348/32349

### Volume Conflicts

Each deployment creates volumes prefixed with `${NODE_NAME}`. Make sure each deployment has a unique `NODE_NAME`.

## Support

For issues or questions about EdgeLake, refer to the EdgeLake documentation:
`/path/to/documentation`
