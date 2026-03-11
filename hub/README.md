# Hub Infrastructure (trv-srv-001)

Centralized services running on the hub server (<hub-tailscale-ip>).

## Services

### EdgeLake Master + Query (`edgelake/`)

Docker-compose deployments for the blockchain coordinator (master) and query execution node.

```bash
cd edgelake
make up-all         # Start master, then query
make status-all     # Check status
make attach query   # Attach to query CLI
```

See [edgelake/README.md](edgelake/README.md) for full documentation.

### Open Horizon Management Hub (`open-horizon/`)

OH management hub configuration, service publishing, and deployment policies.

```bash
cd open-horizon
make -f Makefile.oh help              # Show available commands
make -f Makefile.oh oh-publish-all    # Publish services to Exchange
```

See [open-horizon/QUICKSTART_OH.md](open-horizon/QUICKSTART_OH.md) for quick reference.

## Server Details

| Setting | Value |
|---------|-------|
| Hostname | trv-srv-001 |
| Tailscale IP | <hub-tailscale-ip> |
| Docker Registry | <hub-tailscale-ip>:5000 |
| OH Exchange | http://<hub-tailscale-ip>:3090/v1 |
| OH CSS | http://<hub-tailscale-ip>:9443 |
| ts-store | http://localhost:21080 |

## Also Running

- **ts-store**: System stats collection (see `devices/trv-srv-001/`)
- **Docker Registry**: Private registry at port 5000
