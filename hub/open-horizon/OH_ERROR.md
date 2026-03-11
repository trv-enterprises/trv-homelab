Open Horizon Management Hub - Version Information

  Error Description

  AgBot PostgreSQL Schema Error: pq: null value in column "secret_exists"
  of relation "secrets_pattern_3" violates not-null constraint

  This error occurs when the agbot tries to claim an unowned partition,
  blocking agreement formation.

  Component Versions

  Management Hub (trv-srv-001 / <hub-tailscale-ip>):
  - Exchange API: 2.153.0
    - Container: quay.io/open-horizon/exchange-ubi:2.153.0-testing
  - AgBot: Horizon 2.32.0-1571
    - Container: openhorizon/amd64_agbot:latest
    - Required Min Exchange: 2.90.1
    - Preferred Exchange: 2.110.1
  - CSS (Cloud Sync Service): 1.10.1-1591
    - Container: openhorizon/amd64_cloud-sync-service:1.10.1-1591
  - PostgreSQL: 13.23 (Debian 13.23-1.pgdg13+1)
    - Container: postgres:13
  - Bao (Vault): 2.0
    - Container: quay.io/openbao/openbao-ubi:2.0

  Edge Device (trv-srv-014 / <edge-srv-014-tailscale-ip>):
  - Horizon CLI: 2.31.0-1498
  - Horizon Agent: 2.31.0-1498

  Host OS (Management Hub):
  - Linux (kernel details available if needed)

  Deployment Method

  - Deployed using deploy-mgmt-hub.sh script
  - Source: Open Horizon devops repository
  (/home/<user>/devops/mgmt-hub/)

  Issue Context

  1. Performed clean deployment (removed volumes and redeployed)
  2. Created new trv-services organization successfully
  3. Configured agbot to serve multiple orgs (IBM, myorg, trv-services)
  4. AgBot successfully queries business policies and discovers nodes
  5. Agreement initiation fails with database constraint error
  6. Workload usage record created with ReqsNotMet: true

  Specific Error Log Entry

  E1118 20:12:46.202141       8 agreementbot.go:1489] AgreementBotWorker
  Error claiming an unowned partition, error: pq: null value in column
  "secret_exists" of relation "secrets_pattern_3" violates not-null
  constraint

  Additional Context

  - Error repeats every ~60 seconds
  - AgBot can see and query business policies correctly
  - AgBot identifies compatible nodes successfully
  - Agreement proposal begins but fails at partition claim step
  - Database appears healthy otherwise (accepts connections, basic queries
  work)

  Questions for Product Team

  1. Is this a known issue with Exchange 2.153.0 + AgBot 2.32.0?
  2. Is there a database migration script that should run after deployment?
  3. Could this be related to the -testing suffix on the Exchange image?
  4. Are there any workarounds for the secret_exists column constraint?
  5. Should we downgrade to specific version combinations?