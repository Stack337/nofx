# Private AI Trading Agent Service Design

**Date:** 2026-08-20  
**Status:** Approved for implementation planning  
**Scope:** Private, single-user trading service extending the existing NOFX dashboard and codebase.

## Goal

Build a private dashboard and AI-agent service with VergeX-like operating flow, while keeping the implementation independent, auditable, and safe to run beside the existing VergeX, 9Router, and CLI Proxy services. The first exchange is Bybit USDT Perpetual. Other exchanges may be added through an exchange interface later.

The first release contains the complete Shadow, Paper, and Live execution paths. Live trading is available but never enabled implicitly: an agent must pass health and risk checks and receive an explicit per-agent manual Live confirmation.

## Product decisions

- One user with multiple Bybit accounts.
- Multiple agents, including multiple agents on one account.
- Per-agent strategy, AI provider/model, risk profile, market-data sources, schedule, and execution mode.
- Full Bybit USDT Perpetual universe, filtered by liquidity, volume, spread, and data quality.
- Aggressive risk profile is supported, but hard limits and a kill switch are mandatory.
- SL/TP are configurable per agent; the risk engine can reject unsafe levels.
- Initial access is by VPS IP. HTTPS and stronger public authentication are a later hardening step.
- Dashboard is an extension of the current NOFX dashboard, with a separate AI-agent module.

## Architecture

The service is a modular monolith. Modules communicate through typed internal interfaces and persisted events rather than direct UI-to-exchange calls.

- `agent-core`: schedules cycles, owns agent state, and coordinates the workflow.
- `market-data`: collects and normalizes candles, volume, indicators, funding, open interest, liquidations, and configured public sources.
- `ai500`: ranks and scores candidates using the existing deterministic feature and observation foundation.
- `ai-providers`: adapters for OpenRouter, direct OpenAI, direct DeepSeek, and local OpenAI-compatible endpoints.
- `risk-engine`: validates exposure, sizing, leverage, SL/TP, account limits, kill-switch state, and decision freshness. It has final authority.
- `execution`: Bybit REST/WebSocket adapter, order lifecycle, idempotency, reconciliation, and Shadow/Paper/Live routing.
- `audit`: append-only decision, order, retry, error, and reconciliation records.
- `dashboard-api`: authenticated API surface for the dashboard and operational controls.

The existing AI500 clone remains read-only/shadow foundation until it is connected through the new agent interfaces. Existing VergeX, 9Router, and CLI Proxy processes, ports, and configurations are not modified or restarted.

## AI cycle and data flow

1. The scheduler creates a cycle with a unique cycle ID and deadline.
2. Market data is read from configured providers and validated for freshness and completeness.
3. AI500 scores and ranks eligible symbols; observations and scores are persisted.
4. The selected LLM receives a strict, versioned decision contract containing only normalized market context, AI500 output, current positions, account/risk state, and allowed actions.
5. The model may return `open`, `close`, or `hold` with structured rationale and parameters. Free-form or malformed output is rejected.
6. The risk engine validates the decision and can convert it to `hold`/reject.
7. Shadow records the hypothetical action, Paper simulates fills, and Live submits an exchange order only after the agent's Live gate is enabled.
8. Execution records exchange responses and updates order/position state.
9. The cycle ends with a terminal status and audit event. It cannot remain in `Processing` indefinitely.

## Execution and risk

- Bybit account and position mode are read before order placement; `positionIdx`, reduce-only, side, quantity, tick size, step size, leverage, and margin mode are normalized explicitly.
- Every mutating request has an idempotency key. On timeout or unknown response, the system queries order status before retrying.
- Unknown order state moves the cycle/agent to `needs_reconciliation`; no blind duplicate order is sent.
- Safe transient requests use bounded exponential backoff. Authentication, validation, parameter, and permission errors are terminal until corrected.
- Stop-loss and take-profit placement is verified after entry. Missing protection prevents the cycle from being marked successful and can trigger an emergency policy.
- Exposure, max position count, daily loss, per-trade loss, leverage, notional, symbol concentration, stale data, and provider disagreement are risk checks.
- Kill switch stops new entries immediately and exposes a separately confirmed emergency-close operation.
- Live mode requires explicit per-agent confirmation and is visible in the audit trail.

## Persistence and observability

- PostgreSQL stores users, encrypted account references, agents, strategies, model/provider configuration, schedules, risk profiles, orders, positions, and cycle state.
- Append-only JSONL stores AI500 observations, model decision metadata, audit events, reconciliation events, and technical diagnostics. Secrets, raw API keys, refresh tokens, and unrestricted prompts are never logged.
- Dashboard shows balances, equity, PnL, positions, orders, AI decisions, AI500 score, cycle state, retries, errors, provider/exchange health, and kill-switch/Live-gate state.
- Cycle states include `queued`, `processing`, `completed`, `failed`, `needs_reconciliation`, and `stopped`.
- Each processing operation has a deadline, cancellation path, bounded retries, and a terminal error transition.

## Security and operations

- Bybit credentials are encrypted at rest and decrypted only inside the execution boundary.
- The application implements only read, market-data, account, and trade operations. Withdraw, transfer, and API-key-management methods are absent from the interface and blocked in code.
- Initial deployment binds only the designated private service port and is accessed by VPS IP. Existing services remain untouched.
- Logs have rotation and size limits appropriate for the VPS: 1 vCPU, about 1.9 GB RAM, 4.5 GB swap, and about 6.4 GB currently free disk.
- Backups cover PostgreSQL and audit data; restore procedures are tested before Live activation.
- Health checks cover process, database, Bybit connectivity, market-data freshness, AI provider reachability, disk, memory, and queue age.
- Deployment uses a separate service name, configuration namespace, data directory, and port. No automatic restart of VergeX, 9Router, or CLI Proxy is allowed.

## Release and verification gates

Although the complete execution code is included in the first release, rollout is staged operationally:

1. Build and unit/integration test with exchange calls mocked.
2. Shadow verification with zero private Bybit order calls.
3. Paper verification against live market data and simulated fills.
4. Reconciliation, failure-injection, timeout, malformed-AI-output, and risk-limit tests.
5. Manual Live gate for one agent and one Bybit account.
6. Controlled Live expansion only after audit and PnL/position checks.

Success means that a cycle has observable terminal state, malformed/provider failures do not hang the agent, duplicate orders are prevented, risk checks can block any model decision, and Live orders are possible only through explicit confirmation.

## Out of scope for this implementation plan

- Public multi-user SaaS, billing, or tenant isolation.
- Automatic API-key creation, withdrawals, transfers, or fund management.
- Guaranteed 1:1 reproduction of proprietary VergeX prompts or undisclosed internals.
- GPU/local-model hosting on the current VPS.
- Adding exchanges beyond the Bybit adapter.
- Changing or replacing the existing VergeX, 9Router, or CLI Proxy deployment.

