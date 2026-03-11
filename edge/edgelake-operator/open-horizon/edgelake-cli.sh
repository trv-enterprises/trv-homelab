#!/bin/bash
# EdgeLake CLI Helper for Open Horizon Deployed Containers
#
# Open Horizon creates containers with long names (agreement-id prefix)
# and doesn't support TTY by default. This script helps access the EdgeLake CLI.

set -e

# Find the EdgeLake container (try multiple image patterns)
CONTAINER=$(docker ps --format "{{.ID}}\t{{.Image}}" | grep -i edgelake | head -1 | cut -f1)

if [ -z "$CONTAINER" ]; then
    echo "Error: No EdgeLake container found running"
    echo "Check with: docker ps | grep edgelake"
    exit 1
fi

CONTAINER_NAME=$(docker ps --filter "id=$CONTAINER" --format "{{.Names}}")
echo "Found EdgeLake container: $CONTAINER_NAME"
echo "Container ID: $CONTAINER"
echo ""

# Check if we want to run a specific command or interactive shell
if [ $# -eq 0 ]; then
    echo "Usage:"
    echo "  $0 <command>              - Run EdgeLake command (e.g., 'get status')"
    echo "  $0 shell                  - Open bash shell in container"
    echo ""
    echo "Examples:"
    echo "  $0 'get status'"
    echo "  $0 'get processes'"
    echo "  $0 'blockchain get operator'"
    echo "  $0 shell"
    exit 0
fi

if [ "$1" = "shell" ]; then
    echo "Opening bash shell (type 'exit' to leave)..."
    docker exec -i $CONTAINER bash
else
    # Run EdgeLake command
    # Note: OH containers don't support -t (TTY), only -i (interactive)
    echo "Running command: $@"
    echo ""
    docker exec -i $CONTAINER bash -c "echo '$@' | /app/edgelake_agent"
fi
