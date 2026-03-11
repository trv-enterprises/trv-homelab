# ts-store Local Configuration

Environment-specific settings for ts-store deployment. These files are kept separate from the main repo to avoid committing secrets or personal paths.

## Files

- `.env` - Environment variables for Makefile (Pi deployment settings)
- `Makefile.local` - Reference documentation

## Usage

From the ts-store repo directory:

```bash
# Copy env file
cp ../utilities/tsstore/.env .env

# Build and deploy to Pi
make deploy-pi

# Or just build
make build
```

## What's in .env

```
PI_HOST=<user>@<pi-001-tailscale-ip>      # SSH target
PI_BINARY_PATH=/home/<user>/bin/tsstore
PI_SERVICE=tsstore                   # systemd service name
```

The `.env` file is gitignored in ts-store, so your settings stay local.
