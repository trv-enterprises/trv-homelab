# seaweedfs

Single-node S3-compatible object store (SeaweedFS, Apache-2.0) for
development and automated S3-driver tests. Not a production store: no
replication (`000`), and the data is treated as disposable.

`docker-compose.yml` is fully parameterized; the deploy overlay supplies the
values. `verify.py` checks a bucket-scoped identity against an admin one.

## Why SeaweedFS

Ceph is cluster-scale (about 4 GB RAM per storage daemon). MinIO's community
edition has been source-only since October 2025. SeaweedFS's `weed mini`
runs master, volume server, filer and S3 gateway in one process at about
100 MB idle.

## Things that are deliberate

- **Only the S3 port is published.** `weed mini` also starts the master,
  volume, filer and their gRPC ports, and the filer's IAM gRPC service is
  unauthenticated (it logs so at start): anyone who reaches it can create
  users. They stay inside the container network. WebDAV and the admin UI,
  on by default in mini, are turned off.
- **Identities come from a static file (`-s3.config`) and are read once, at
  start.** Changing an identity means replacing the file and restarting the
  container. Pointing at a config file turns authentication on even if the
  file yields no identity (everything is then denied), so a bad render fails
  closed. Anonymous requests get 403.
- **Bucket scope is in the action names.** `Read:<bucket>`, `Write:<bucket>`,
  `List:<bucket>`, `Tagging:<bucket>` per bucket, and no `Admin`: such an
  identity cannot create or delete buckets, cannot touch any other bucket,
  and `ListBuckets` returns only its own. Verified on 4.52 with `verify.py`.
- **Buckets are declared, not created by hand.** `-bucket=` creates each
  listed bucket at start if it is missing and leaves existing ones alone.
- **Runs as the host user that owns the mounts.** As root, the image's
  entrypoint chowns `/data` to its own `seaweed` user; as non-root it execs
  `weed` directly. Running as the owner keeps the identity file at mode 0600
  on the host. The data dir must exist, owned by that user, before the
  container starts — a bind-mount path Docker creates is root-owned.
- **The config is a directory mount.** Replacing a file gives it a new inode,
  which a single-file bind mount never sees (same trap as Marshal's
  `rules.yaml`).
- **Path-style addressing.** Clients must use path-style
  (`<endpoint>/<bucket>/<key>`); there is no wildcard DNS for
  virtual-hosted buckets. The region string is not checked; send
  `us-east-1`.

## TLS

The gateway speaks plain HTTP. For TLS, front it with `tailscale serve`
(HTTPS on a tailnet port, proxying to `127.0.0.1:<s3 port>`). SigV4 signs the
`Host` header, so whatever fronts it must pass `Host` through unchanged.

## Upgrades

Releases are frequent (several a month). Pin an exact tag; never `latest`.
The `_large_disk` image variants (5-byte offsets, volumes over 32 GB) are not
needed at mini's 128 MB volume size and are not interchangeable with the
plain image on existing data.
