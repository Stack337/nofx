# Private AI Trading Agent Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a private, multi-agent Bybit USDT Perpetual service to NOFX with AI500 ranking, pluggable LLM decisions, explicit Shadow/Paper/Live modes, and a risk-first execution lifecycle.

**Architecture:** Implement a modular monolith in the existing Go backend and React/Vite dashboard. New agent modules depend on narrow interfaces for market data, AI providers, risk, persistence, and exchange execution; the existing trader and AI500 code is adapted through those interfaces rather than duplicated. Live order submission remains behind a per-agent gate and is never enabled by configuration alone.

**Tech Stack:** Go, existing GORM/PostgreSQL store, existing Bybit client/trader packages, JSONL append-only diagnostics, React/TypeScript/Vite, Vitest, Go `testing`.

## Global Constraints

- First exchange is Bybit USDT Perpetual; the exchange interface must permit later adapters.
- Modes are Shadow, Paper, and Live; Live requires explicit per-agent confirmation.
- Risk engine has final authority over every model decision.
- Withdraw, transfer, and API-key-management operations are not implemented.
- Unknown order status requires reconciliation before retry; duplicate mutation is prohibited.
- Processing cycles have deadlines, bounded retries, cancellation, and terminal states.
- Existing VergeX, 9Router, and CLI Proxy services, ports, data, and processes must not be restarted or modified.
- No real Bybit order is sent by tests, Shadow, or Paper.
- Never log API keys, refresh tokens, or raw credentials.

---

## Repository map and boundaries

Create these focused packages/files:

- `agent/`: cycle state machine, scheduler, agent configuration, execution mode, and lifecycle interfaces.
- `market/agent_data.go`: normalized agent market snapshot assembled from existing market primitives.
- `ai500/agent_ranker.go`: adapter from existing AI500 observations to ranked candidates.
- `provider/agent_provider.go`: strict decision contract and provider adapter boundary.
- `risk/`: risk policy, sizing, protection validation, kill switch, and decision authorization.
- `execution/`: exchange-neutral order lifecycle and Shadow/Paper/Live router.
- `trader/bybit/agent_exchange.go`: Bybit adapter using existing authenticated client patterns.
- `store/agent.go`, `store/agent_cycle.go`, `store/agent_order.go`: durable agent state and idempotent cycle/order records.
- `audit/`: append-only audit writer with rotation-safe interface.
- `api/handler_agents.go`, `api/handler_agent_cycles.go`: dashboard API endpoints.
- `web/src/components/agents/`: agent dashboard, decisions, health, and Live confirmation UI.

Reuse existing `ai500`, `market`, `trader/bybit`, `store`, crypto, auth, and route-registration conventions after inspecting their exported interfaces. Do not fold the new state machine into the existing `trader/auto_trader_*` files.

### Task 1: Define agent domain contracts and terminal cycle state machine

**Files:**
- Create: `agent/types.go`
- Create: `agent/cycle.go`
- Create: `agent/cycle_test.go`
- Modify: `go.mod` only if an existing dependency is required; do not add a new framework.

**Interfaces:**
- Produce `type Mode string` with `ModeShadow`, `ModePaper`, `ModeLive`.
- Produce `type CycleStatus string` with `queued`, `processing`, `completed`, `failed`, `needs_reconciliation`, `stopped`.
- Produce `type DecisionAction string` with `open`, `close`, `hold`.
- Produce `type AgentCycle struct { ID, AgentID string; Status CycleStatus; Attempt int; Deadline time.Time; ErrorCode string }`.
- Produce `type CycleRunner interface { Run(ctx context.Context, cycle AgentCycle) error }`.

- [ ] **Step 1: Write failing state-transition tests** for legal terminal transitions, timeout cancellation, and rejection of a second terminal transition.
- [ ] **Step 2: Run `go test ./agent -run TestCycle -v`** and verify failure because the package/contracts do not exist.
- [ ] **Step 3: Implement the contracts and a mutex-protected transition function** that accepts only `queued -> processing` and `processing -> completed|failed|needs_reconciliation|stopped`.
- [ ] **Step 4: Add a deadline wrapper** that returns a typed timeout error and transitions processing cycles to `failed` rather than leaving them pending.
- [ ] **Step 5: Run `go test ./agent -v`; commit** with `feat: define agent cycle lifecycle`.

### Task 2: Add PostgreSQL agent, cycle, and audit persistence

**Files:**
- Create: `store/agent.go`
- Create: `store/agent_cycle.go`
- Create: `store/agent_order.go`
- Create: `store/agent_test.go`
- Create: `audit/audit.go`
- Create: `audit/audit_test.go`
- Modify: `store/store.go` and existing migration/bootstrap registration.

**Interfaces:**
- `type AgentRepository interface { Create(ctx context.Context, a Agent) error; Get(ctx context.Context, id string) (Agent, error); UpdateMode(ctx context.Context, id string, mode Mode, liveConfirmed bool) error; List(ctx context.Context) ([]Agent, error) }`.
- `type CycleRepository interface { Create(ctx context.Context, c AgentCycle) error; Transition(ctx context.Context, id string, from, to CycleStatus, errCode string) error; Get(ctx context.Context, id string) (AgentCycle, error) }`.
- `type IdempotencyRepository interface { Reserve(ctx context.Context, key string) (bool, error); Complete(ctx context.Context, key, exchangeOrderID string) error }`.
- `type AuditWriter interface { Append(ctx context.Context, event AuditEvent) error }`.

- [ ] **Step 1: Write repository tests** using the existing store test database pattern for unique agent IDs, optimistic cycle transitions, idempotency uniqueness, and Live confirmation persistence.
- [ ] **Step 2: Run the focused tests** and verify they fail before models/repositories are present.
- [ ] **Step 3: Add GORM models and migrations** with indexes on `(agent_id, created_at)`, `(cycle_id, status)`, and unique `idempotency_key`.
- [ ] **Step 4: Implement transaction-safe repository methods**; a failed transition must not update the audit row or cycle status partially.
- [ ] **Step 5: Implement JSONL audit append with one serialized writer and bounded error return**; commit `feat: persist agent state and audit events`.

### Task 3: Build normalized market snapshot and AI/provider contracts

**Files:**
- Create: `market/agent_data.go`
- Create: `market/agent_data_test.go`
- Create: `ai500/agent_ranker.go`
- Create: `ai500/agent_ranker_test.go`
- Create: `provider/agent_provider.go`
- Create: `provider/agent_provider_test.go`

**Interfaces:**
- `type MarketSnapshot struct { Symbol string; Timestamp time.Time; Candles []Candle; Volume, Spread, Funding, OpenInterest float64; Liquidations []Liquidation; Fresh bool }`.
- `type MarketSource interface { Snapshot(ctx context.Context, symbol string, cfg SourceConfig) (MarketSnapshot, error) }`.
- `type CandidateRanker interface { Rank(ctx context.Context, snapshot []MarketSnapshot, cfg RankConfig) ([]Candidate, error) }`.
- `type DecisionRequest struct { CycleID, AgentID string; Candidates []Candidate; Positions []PositionView; Risk RiskView; AllowedActions []DecisionAction }`.
- `type DecisionResponse struct { Action DecisionAction; Symbol string; Side string; Quantity float64; Leverage int; StopLoss, TakeProfit *float64; Confidence float64; Rationale string }`.
- `type Provider interface { Decide(ctx context.Context, req DecisionRequest) (DecisionResponse, ProviderMeta, error) }`.

- [ ] **Step 1: Write tests** for stale/incomplete snapshots, deterministic candidate ordering, strict action validation, and malformed/empty provider responses.
- [ ] **Step 2: Run focused tests** and verify failure.
- [ ] **Step 3: Implement normalization over existing `market` and AI500 primitives**; reject snapshots older than the configured freshness window.
- [ ] **Step 4: Implement the provider contract parser** with bounded request timeout and typed errors for empty JSON, missing action, invalid symbol, and unsupported tool/function output.
- [ ] **Step 5: Run `go test ./market ./ai500 ./provider -v`; commit** `feat: add agent market and AI contracts`.

### Task 4: Implement risk engine and manual safety gates

**Files:**
- Create: `risk/types.go`
- Create: `risk/engine.go`
- Create: `risk/engine_test.go`
- Create: `risk/kill_switch.go`
- Create: `risk/kill_switch_test.go`

**Interfaces:**
- `type Policy struct { MaxLeverage int; MaxNotional, MaxDailyLoss, MaxTradeLoss float64; MaxPositions int; RequireProtection bool }`.
- `type Engine interface { Authorize(ctx context.Context, d DecisionResponse, account AccountRisk, policy Policy) (AuthorizedDecision, error) }`.
- `type KillSwitch interface { Enabled(ctx context.Context, agentID string) (bool, error); Enable(ctx context.Context, agentID, reason string) error; Disable(ctx context.Context, agentID string, confirmation string) error }`.

- [ ] **Step 1: Write failing tests** for each hard limit, unsafe SL/TP, stale decision, kill switch, and `hold` pass-through.
- [ ] **Step 2: Run `go test ./risk -v`** and verify failure.
- [ ] **Step 3: Implement pure validation and sizing functions**; risk rejection returns a stable code such as `RISK_MAX_NOTIONAL` or `RISK_PROTECTION_REQUIRED`.
- [ ] **Step 4: Implement persisted kill-switch state** and require an explicit confirmation token for disabling it.
- [ ] **Step 5: Run focused and full Go tests; commit** `feat: add risk authorization and kill switch`.

### Task 5: Implement exchange-neutral execution and Bybit adapter

**Files:**
- Create: `execution/types.go`
- Create: `execution/router.go`
- Create: `execution/router_test.go`
- Create: `trader/bybit/agent_exchange.go`
- Create: `trader/bybit/agent_exchange_test.go`
- Modify: `trader/bybit/trader.go` only to expose an existing safe client seam if necessary.

**Interfaces:**
- `type Exchange interface { Account(ctx context.Context) (AccountState, error); Instrument(ctx context.Context, symbol string) (InstrumentSpec, error); Place(ctx context.Context, req OrderRequest) (OrderResult, error); GetOrder(ctx context.Context, symbol, orderID string) (OrderState, error); PositionMode(ctx context.Context, symbol string) (PositionMode, error) }`.
- `type Router interface { Execute(ctx context.Context, mode Mode, d AuthorizedDecision, cycleID string) (ExecutionResult, error) }`.

- [ ] **Step 1: Write router tests** proving Shadow makes zero exchange mutation calls, Paper creates simulated fills, and Live requires `liveConfirmed == true`.
- [ ] **Step 2: Write Bybit request tests** for one-way/hedge `positionIdx`, symbol filters, quantity/price rounding, reduce-only, and protection order parameters.
- [ ] **Step 3: Run focused tests** and verify failure.
- [ ] **Step 4: Implement router and Bybit adapter** using existing authentication and request signing patterns; never add withdrawal/transfer methods.
- [ ] **Step 5: Implement unknown-response reconciliation**: query order state before any retry and return `needs_reconciliation` when state remains unknown.
- [ ] **Step 6: Run `go test ./execution ./trader/bybit -v`; commit** `feat: add guarded Bybit execution router`.

### Task 6: Orchestrate scheduled AI cycles

**Files:**
- Create: `agent/service.go`
- Create: `agent/service_test.go`
- Create: `agent/scheduler.go`
- Create: `agent/scheduler_test.go`
- Modify: `main.go` or the existing service bootstrap to register the service without changing current service startup semantics.

**Interfaces:**
- `type Service struct { Agents AgentRepository; Cycles CycleRepository; Market MarketSource; Ranker CandidateRanker; Provider Provider; Risk Engine; Router Router; Audit AuditWriter }`.
- `func (s *Service) RunCycle(ctx context.Context, agentID string) (AgentCycle, error)`.
- `func (s *Service) Start(ctx context.Context) error` and `Stop(ctx context.Context) error`.

- [ ] **Step 1: Write orchestration tests** for successful hold, risk rejection, provider timeout, exchange timeout, unknown order, and cycle deadline.
- [ ] **Step 2: Run tests** and verify failure.
- [ ] **Step 3: Implement the sequence** `queued -> processing -> snapshot -> rank -> decide -> authorize -> execute -> audit -> terminal` with `defer`-based terminalization.
- [ ] **Step 4: Add bounded retry only around classified safe operations** and prevent overlapping cycles for one agent.
- [ ] **Step 5: Add scheduler tests** for interval, disabled agent, stop cancellation, and queue age.
- [ ] **Step 6: Run `go test ./agent ./...`; commit** `feat: orchestrate guarded agent cycles`.

### Task 7: Add dashboard API and explicit Live controls

**Files:**
- Create: `api/handler_agents.go`
- Create: `api/handler_agent_cycles.go`
- Create: `api/handler_agents_test.go`
- Modify: `api/route_registry.go`.

**Endpoints:**
- `GET /api/agents`
- `POST /api/agents`
- `GET /api/agents/{id}`
- `POST /api/agents/{id}/start`
- `POST /api/agents/{id}/stop`
- `POST /api/agents/{id}/kill-switch`
- `POST /api/agents/{id}/live-confirmation`
- `GET /api/agents/{id}/cycles`
- `GET /api/agent-cycles/{id}`

- [ ] **Step 1: Write handler tests** for auth, validation, mode changes, Live confirmation, idempotent start/stop, and safe error serialization.
- [ ] **Step 2: Run focused API tests** and verify failure.
- [ ] **Step 3: Register routes through the existing registry** and connect handlers to `agent.Service`.
- [ ] **Step 4: Ensure API responses expose status/error codes but never credentials or raw prompts.**
- [ ] **Step 5: Run `go test ./api -v`; commit** `feat: expose agent lifecycle API`.

### Task 8: Add the AI-agent dashboard module

**Files:**
- Create: `web/src/components/agents/AgentListPage.tsx`
- Create: `web/src/components/agents/AgentDetailPage.tsx`
- Create: `web/src/components/agents/LiveGateDialog.tsx`
- Create: `web/src/components/agents/AgentHealthPanel.tsx`
- Create: `web/src/components/agents/agentApi.ts`
- Create: `web/src/components/agents/AgentListPage.test.tsx`
- Modify: `web/src/App.tsx` and existing navigation only for new routes.

- [ ] **Step 1: Write UI tests** for mode badges, Processing timeout/error display, kill switch, and Live confirmation requiring explicit confirmation text.
- [ ] **Step 2: Run `cd web; npm test -- --run`** and verify the new tests fail.
- [ ] **Step 3: Implement API client and pages** showing balance/equity/PnL, positions/orders, AI500 scores, decisions, cycle retries/errors, provider/exchange health, and controls.
- [ ] **Step 4: Add a confirmation dialog that cannot enable Live by a normal start click.**
- [ ] **Step 5: Run `npm test -- --run` and `npm run build`; commit** `feat: add AI agent dashboard`.

### Task 9: Deployment isolation, migrations, and operational checks

**Files:**
- Create: `docker-compose.agent.yml`
- Create: `docker/agent.Dockerfile`
- Create: `docs/operations/private-agent-service.md`
- Modify: `.env.example`, service/bootstrap registration, and migration registration.
- Test: deployment configuration validation script under `scripts/agent_preflight.*` following existing shell conventions.

- [ ] **Step 1: Add configuration tests** for a distinct port, disabled Live by default, bounded timeouts/retries, log limits, and required encryption key.
- [ ] **Step 2: Implement isolated service configuration** with a separate data/log directory and no dependencies that restart existing services.
- [ ] **Step 3: Add preflight checks** for database, disk, memory, Bybit connectivity, provider health, stale market data, and occupied ports.
- [ ] **Step 4: Document backup/restore, Shadow/Paper/Live promotion, kill switch, reconciliation, and rollback.**
- [ ] **Step 5: Run `go test ./...`, `go vet ./...`, `cd web; npm test -- --run`, and `npm run build`; commit** `chore: isolate agent service deployment`.

### Task 10: Failure-injection and Live promotion verification

**Files:**
- Create: `agent/live_promotion_test.go`
- Create: `execution/failure_injection_test.go`
- Create: `api/agent_live_gate_test.go`
- Create: `docs/operations/live-promotion-checklist.md`

- [ ] **Step 1: Add tests** for empty JSON, truncated streams, HTTP 429/404/500, tunnel timeout, Bybit 34040, position-mode mismatch, missing protection, duplicate retry, and unknown order state.
- [ ] **Step 2: Verify every failure has a terminal cycle status and stable error code.**
- [ ] **Step 3: Verify Shadow and Paper make zero private Bybit mutation calls.**
- [ ] **Step 4: Verify Live cannot start without health checks, risk checks, and explicit per-agent confirmation.**
- [ ] **Step 5: Run the complete validation suite and record outputs in the checklist; commit** `test: verify agent live promotion gates`.

## Validation commands

From `E:/project/hermes/nofx-source`:

```powershell
go test ./...
go vet ./...
Set-Location web
npm test -- --run
npm run build
```

Expected result: all Go tests and vet pass, frontend tests pass, and the production frontend build completes. Any test that would place a real Bybit order must be blocked by a mock exchange or an explicit integration-test build tag.

