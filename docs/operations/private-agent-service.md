# Private Agent Service Operations

This deployment is separate from VergeX, 9Router, CLI Proxy, and the existing NOFX containers. Never use `docker compose down` without `-f docker-compose.agent.yml` and never reuse their ports or volumes.

## Preflight

Create a private `.env.agent` with `AGENT_DB_PASSWORD` and a random `AGENT_ENCRYPTION_KEY`. Keep `PRIVATE_AGENT_LIVE_DEFAULT=false`.

```sh
set -a
. ./.env.agent
set +a
./scripts/agent_preflight.sh
```

Preflight validates secrets, port `18080`, free disk, Docker, and Compose syntax. It starts nothing.

## Start and stop

```sh
docker compose --env-file .env.agent -f docker-compose.agent.yml up -d --build
docker compose --env-file .env.agent -f docker-compose.agent.yml stop private-agent
```

The service binds to `127.0.0.1:18080` unless `AGENT_BIND_IP` is explicitly changed. Expose it only through a separately configured authenticated reverse proxy.

## Promotion

1. Keep every new agent in Shadow.
2. Verify terminal cycles, provider health, AI500 observations, and zero exchange mutation calls.
3. Switch to Paper and verify simulated positions and protection logic.
4. Run failure-injection tests and reconcile all unknown orders.
5. Confirm Live for exactly one agent using `ENABLE LIVE <agent_id>`.
6. Enable more agents only after checking Bybit positions, orders, audit events, and PnL.

## Kill switch and reconciliation

`POST /api/agents/<id>/stop` enables the kill switch. Disabling it requires `DISABLE <agent_id>`. An order with unknown status moves its cycle to `needs_reconciliation`; inspect Bybit order history before resuming the agent.

## Backup and restore

Back up the PostgreSQL volume with `pg_dump` and copy `/data/ai500-observations.jsonl` plus `/data/audit/events.jsonl`. Restore into a stopped private-agent deployment, run it in Shadow, and verify cycle/order state before allowing Paper or Live.

