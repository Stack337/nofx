# NOFX VPS Deployment Design

## Objective

Deploy the official stable NOFX release on `176.124.198.44` without changing or stopping the existing 9Router, CLI Proxy, VergeX adapters, Kiro, or trading services.

## Deployment

- Use the official prebuilt images referenced by the `main` branch `docker-compose.prod.yml`.
- Install under `/opt/nofx` with persistent SQLite data in `/opt/nofx/data`.
- Publish the frontend on host port `3001` and backend API on host port `8081`; both ports are currently unused.
- Generate unique JWT, AES-256, and RSA secrets on the VPS. Store them only in `/opt/nofx/.env` with mode `0600`.
- Use timezone `Asia/Novosibirsk`.
- Do not configure exchange or AI credentials during installation.

## Isolation and Safety

- Use the dedicated Compose project, network, containers, and data directory defined by NOFX.
- Do not prune Docker images, remove existing containers, or alter existing reverse proxies and tunnels.
- Keep the NOFX database and generated secrets across updates and restarts.
- Allow external access only to the frontend port `3001`; keep backend port `8081` blocked by UFW unless direct API access is later required.

## Verification

- Confirm both NOFX containers are running and healthy.
- Confirm `http://127.0.0.1:8081/api/health` succeeds on the VPS.
- Confirm the frontend responds through `http://176.124.198.44:3001`.
- Recheck existing 9Router, adapter, tunnel, and Docker services after deployment.

## Rollback

Run `docker compose down` in `/opt/nofx`. This removes the NOFX containers and network while preserving `/opt/nofx/data` and `/opt/nofx/.env`.
