# NOFX VPS Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run the official stable NOFX frontend and backend on the existing VPS without disrupting its current services.

**Architecture:** Deploy the official production Docker images under `/opt/nofx` with a dedicated Compose network and persistent host-mounted SQLite data. Generate secrets on the VPS, expose the frontend on `3001`, expose the backend container on host `8081` for local health checks, and allow only `3001` through UFW.

**Tech Stack:** Docker Engine 28, Docker Compose v5, NOFX official GHCR images, SQLite, UFW.

## Global Constraints

- Use official prebuilt images from the stable `main` branch production Compose file.
- Preserve all existing containers, services, proxies, databases, and credentials.
- Store generated secrets only in `/opt/nofx/.env` with mode `0600`.
- Use frontend port `3001`, backend port `8081`, and timezone `Asia/Novosibirsk`.
- Do not configure AI or exchange credentials during deployment.

---

### Task 1: Stage isolated production configuration

**Files:**
- Create: `/opt/nofx/docker-compose.yml`
- Create: `/opt/nofx/.env`
- Create: `/opt/nofx/data/`

**Interfaces:**
- Consumes: official `docker-compose.prod.yml` from the NOFX `main` branch.
- Produces: a valid Compose project with `NOFX_FRONTEND_PORT=3001` and `NOFX_BACKEND_PORT=8081`.

- [ ] **Step 1: Confirm target paths and ports are unused**

Run: `test ! -e /opt/nofx && ! ss -ltnH | grep -Eq ':(3001|8081) '`
Expected: exit code `0`.

- [ ] **Step 2: Download and validate production Compose configuration**

Run: `curl -fsSL https://raw.githubusercontent.com/NoFxAiOS/nofx/main/docker-compose.prod.yml -o /opt/nofx/docker-compose.yml && docker compose -f /opt/nofx/docker-compose.yml config --quiet`
Expected: exit code `0`.

- [ ] **Step 3: Generate deployment secrets**

Create `/opt/nofx/.env` with generated `JWT_SECRET`, `DATA_ENCRYPTION_KEY`, and single-line `RSA_PRIVATE_KEY`; also set `NOFX_FRONTEND_PORT=3001`, `NOFX_BACKEND_PORT=8081`, `TZ=Asia/Novosibirsk`, and `TRANSPORT_ENCRYPTION=false`.

- [ ] **Step 4: Validate permissions without printing secrets**

Run: `stat -c '%U:%G %a' /opt/nofx/.env`
Expected: `root:root 600`.

### Task 2: Start and verify NOFX

**Files:**
- Read: `/opt/nofx/docker-compose.yml`
- Read: `/opt/nofx/.env`

**Interfaces:**
- Consumes: the Compose project created by Task 1.
- Produces: healthy `nofx-trading` and `nofx-frontend` containers.

- [ ] **Step 1: Pull official images**

Run: `cd /opt/nofx && docker compose pull`
Expected: both official GHCR images download successfully.

- [ ] **Step 2: Start the isolated stack**

Run: `cd /opt/nofx && docker compose up -d`
Expected: `nofx-trading` and `nofx-frontend` are created and running.

- [ ] **Step 3: Wait for readiness by condition**

Poll `http://127.0.0.1:8081/api/health` for up to 120 seconds.
Expected: HTTP `200` before timeout.

- [ ] **Step 4: Permit frontend access**

Run: `ufw allow 3001/tcp comment 'NOFX frontend'`
Expected: rule is present for IPv4 and IPv6; port `8081` remains blocked externally.

- [ ] **Step 5: Verify frontend and isolation**

Run local and public HTTP probes for port `3001`, list Compose container health, and recheck `9router.service`, `vergex-9router-adapter.service`, and `cloudflared-9router-adapter.service`.
Expected: NOFX responds, both NOFX containers are running/healthy, and all existing services remain active.

### Task 3: Record operational handoff

**Files:**
- Create: `/opt/nofx/DEPLOYMENT.md`

**Interfaces:**
- Consumes: verified deployment state.
- Produces: update, logs, restart, and rollback commands without secrets.

- [ ] **Step 1: Write operational commands**

Document `docker compose pull && docker compose up -d`, `docker compose logs`, `docker compose restart`, and `docker compose down`, plus the public frontend URL and local backend health URL.

- [ ] **Step 2: Verify handoff contains no secret values**

Run: `grep -E 'JWT_SECRET=|DATA_ENCRYPTION_KEY=|RSA_PRIVATE_KEY=' /opt/nofx/DEPLOYMENT.md`
Expected: no matches.
