# EdgeLake MCP Server Build and Deployment

The `Makefile` provides a flexible workflow for building and deploying the EdgeLake MCP server across different environments.

## Quick Start

```bash
# Build locally for arm64
make build arch=arm64

# Build on remote VM for amd64
make build arch=amd64

# Deploy to target host
make deploy

# Build and deploy in one command
make all arch=arm64
```

## Parameters

- **`arch`** - Target architecture for the build
  - `arm64` - Build locally on Mac (default)
  - `amd64` - Build on remote VM via rsync + SSH

## Available Targets

- `make build` - Build Docker image (location depends on arch)
- `make deploy` - Sync docker-compose and deploy
- `make all` - Build and deploy in sequence
- `make up-all` - Start all nodes
- `make down-all` - Stop all nodes
- `make attach <type>` - Attach to node CLI
- `make logs <type>` - Show container logs
- `make load-data` - Load sample data to operator node
- `make info` - Show current configuration
- `make help` - Display usage information

## Related

- EdgeLake source: `/path/to/EdgeLake`
- Docker compose: `/path/to/docker-compose`
- Data loader: `../data-loader/`
