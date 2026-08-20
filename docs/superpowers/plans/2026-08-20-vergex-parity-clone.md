# VergeX Parity Trading Clone Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a shadow-first Bybit trading clone whose cycle, AI/tool protocol, risk gates, persistence, API and dashboard match the observed VergeX behavior.

**Architecture:** Add a bounded parity layer around the existing NOFX domain instead of modifying the live VergeX services. The cycle orchestrator persists every state transition, the AI adapter normalizes Chat Completions/Responses streams, and the exchange layer is tested against a deterministic Bybit mock before any live credentials are accepted.

**Tech Stack:** Go, existing NOFX kernel/trader/store packages, SQLite-backed stores, Gin API, existing web frontend, Node fixture tests for the deployed adapter where applicable.

## Global Constraints

- Shadow mode is the default and must not send private Bybit order requests.
- Live Bybit activation requires a separate explicit user confirmation after parity tests pass.
- Do not stop or reconfigure the existing VergeX/9Router/CLI Proxy services.
- Never print, commit, persist in plaintext, or include in prompts any API key, exchange secret, OAuth token or private key.
- Every cycle must leave `Processing` within its deadline by persisting `completed` or `failed`.
- Bybit one-way mode uses `positionIdx=0`; close orders are reduce-only.

---

### Task 1: Create the parity domain and cycle state machine

**Files:**
- Create: `nofx-source/parity/domain.go`
- Create: `nofx-source/parity/cycle_state.go`
- Test: `nofx-source/parity/cycle_state_test.go`

**Interfaces:**
- Produces `CycleState`, `CycleEvent`, `CycleError`, and validated transitions for the orchestrator.
- Consumes no exchange or AI implementation.

- [ ] **Step 1: Write the failing transition tests**

Test `scheduled -> collecting_context -> analysis_round -> tool_round -> validating -> executing -> synchronizing -> completed`, rejects skipping states, and allows every active state to transition to `failed`.

- [ ] **Step 2: Run the focused test and verify failure**

Run: `go test ./parity -run TestCycleTransition -v`
Expected: FAIL because the parity package and state implementation do not yet exist.

- [ ] **Step 3: Implement the transition table**

Use a map keyed by current state and event. Return a typed error containing current state, event and cycle ID. Do not permit transitions out of `completed` or `failed`.

- [ ] **Step 4: Run the focused test**

Run: `go test ./parity -run TestCycleTransition -v`
Expected: PASS.

- [ ] **Step 5: Commit**

Run: `git add parity/domain.go parity/cycle_state.go parity/cycle_state_test.go && git commit -m "feat: add parity cycle state machine"`

### Task 2: Add persisted cycle and diagnostic records

**Files:**
- Modify: `nofx-source/store/store.go`
- Create: `nofx-source/store/parity_cycle.go`
- Create: `nofx-source/store/parity_cycle_test.go`

**Interfaces:**
- Produces `ParityCycleStore.Create`, `Transition`, `Complete`, `Fail`, and `Get`.
- Consumes the types from `parity/cycle_state.go`.

- [ ] **Step 1: Write tests for durable transitions and correlation IDs**

Create a cycle, transition it twice, reload it from a fresh store handle, and assert state, timestamps, error code and correlation ID are preserved.

- [ ] **Step 2: Run the test to verify failure**

Run: `go test ./store -run TestParityCyclePersistence -v`
Expected: FAIL because the store and schema are absent.

- [ ] **Step 3: Implement schema and store methods**

Add a table keyed by cycle ID with agent ID, state, correlation ID, started/updated/completed timestamps, error code/message, shadow-mode flag and JSON metadata. Use parameterized SQL and transactions for transitions.

- [ ] **Step 4: Run focused and package tests**

Run: `go test ./store -run TestParityCyclePersistence -v && go test ./store`
Expected: PASS.

- [ ] **Step 5: Commit**

Run: `git add store/store.go store/parity_cycle.go store/parity_cycle_test.go && git commit -m "feat: persist parity cycle state"`

### Task 3: Implement deterministic VergeX risk validation

**Files:**
- Create: `nofx-source/parity/risk.go`
- Create: `nofx-source/parity/risk_test.go`

**Interfaces:**
- Produces `RiskConfig`, `DecisionInput`, `RiskResult`, and `ValidateDecision`.
- Consumes normalized decisions and account/market snapshots.

- [ ] **Step 1: Add fixtures for accepted/rejected actions**

Cover long/short SL ordering, minimum general and BTC/ETH size, leverage clamping, position caps, minimum 3:1 reward/risk, stale-price open blocking, and allowed close actions on degraded price data.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./parity -run TestRisk -v`
Expected: FAIL because the risk engine is absent.

- [ ] **Step 3: Implement pure validation**

Keep the function side-effect free. Return a normalized decision plus reason codes such as `invalid_sl_tp`, `position_cap`, `stale_price`, or `risk_reward_too_low`; never silently turn an invalid open into a live order.

- [ ] **Step 4: Run focused tests**

Run: `go test ./parity -run TestRisk -v`
Expected: PASS.

- [ ] **Step 5: Commit**

Run: `git add parity/risk.go parity/risk_test.go && git commit -m "feat: add deterministic trading risk gates"`

### Task 4: Build AI stream normalization and mandatory tool-call handling

**Files:**
- Create: `nofx-source/parity/ai_protocol.go`
- Create: `nofx-source/parity/ai_protocol_test.go`
- Reuse fixtures from: `E:/project/hermes/.tools/vergex-prompt-capture.json`

**Interfaces:**
- Produces `AITransport`, `StreamNormalizer`, `AnalysisText`, `ToolCall`, and typed protocol errors.
- Supports initial text-only streaming followed by required tool streaming.

- [ ] **Step 1: Add failing SSE fixtures**

Test OpenRouter comments, duplicate `[DONE]`, a complete function-call argument stream, missing completed event, completed event with empty output, malformed JSON, HTTP 429, HTTP 404 and HTTP 500.

- [ ] **Step 2: Run protocol tests to verify failure**

Run: `go test ./parity -run TestAIProtocol -v`
Expected: FAIL because normalizer and tool-round client are absent.

- [ ] **Step 3: Implement the smallest normalizer**

Ignore only SSE comment lines, emit one terminal sentinel, validate function-call argument JSON, and require a completed event. Permit output repair only when a complete function-call event has already been observed; otherwise return `incomplete_stream`.

- [ ] **Step 4: Implement required-tool retry policy**

Retry text/no-tool responses up to three rounds. On the final round send only a safe `hold` tool with required tool choice. Bound each round and the whole cycle with context deadlines.

- [ ] **Step 5: Run protocol tests and commit**

Run: `go test ./parity -run TestAIProtocol -v`
Expected: PASS.

Run: `git add parity/ai_protocol.go parity/ai_protocol_test.go && git commit -m "feat: normalize AI streams and require tool calls"`

### Task 5: Add market/account snapshot interfaces and fixtures

**Files:**
- Create: `nofx-source/parity/context.go`
- Create: `nofx-source/parity/context_test.go`

**Interfaces:**
- Produces `ContextProvider`, `MarketSnapshot`, `AccountSnapshot`, `PositionSnapshot`, candidate ordering and prompt input DTOs.
- Consumes existing NOFX trader/provider interfaces without placing orders.

- [ ] **Step 1: Test deterministic candidate ordering and position-first fetching**

Use fake providers to assert active positions are fetched before candidates, invalid candidates are pruned, forming candles are marked live, and duplicate symbols normalize to one canonical symbol.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./parity -run TestContext -v`
Expected: FAIL because the snapshot layer is absent.

- [ ] **Step 3: Implement the snapshot DTOs and fakeable provider boundary**

No prompt construction should call an exchange directly; all values arrive through the snapshot boundary with source timestamp and health metadata.

- [ ] **Step 4: Run focused tests and commit**

Run: `go test ./parity -run TestContext -v`
Expected: PASS.

Run: `git add parity/context.go parity/context_test.go && git commit -m "feat: add deterministic market context boundary"`

### Task 6: Implement Bybit mock contract and shadow execution

**Files:**
- Create: `nofx-source/parity/execution.go`
- Create: `nofx-source/parity/bybit_mock_test.go`
- Modify: `nofx-source/trader/bybit/trader_orders.go` only after mock parity passes

**Interfaces:**
- Produces `ExecutionPort`, `ShadowExecution`, and `BybitOrderRequest`.
- Consumes risk-approved decisions and returns normalized order/fill results.

- [ ] **Step 1: Write mock contract assertions**

Assert opening orders use linear category, market order, one-way `positionIdx=0`; closes add `reduceOnly=true`; SL/TP are conditional reduce-only orders; stale cancellation is idempotent; fills use actual exchange values.

- [ ] **Step 2: Run mock tests to verify failure**

Run: `go test ./parity -run TestBybitContract -v`
Expected: FAIL because the execution port is absent.

- [ ] **Step 3: Implement shadow execution**

Record intended requests and simulated fills without creating an authenticated HTTP client. Make the default constructor shadow-only.

- [ ] **Step 4: Run mock tests**

Run: `go test ./parity -run TestBybitContract -v`
Expected: PASS and zero private exchange requests.

- [ ] **Step 5: Commit**

Run: `git add parity/execution.go parity/bybit_mock_test.go && git commit -m "feat: add shadow Bybit execution contract"`

### Task 7: Assemble the cycle runner and watchdog

**Files:**
- Create: `nofx-source/parity/runner.go`
- Create: `nofx-source/parity/runner_test.go`

**Interfaces:**
- Produces `Runner.RunCycle` and status snapshots consumed by the API.
- Consumes cycle store, context provider, AI transport, risk engine and execution port.

- [ ] **Step 1: Write failure-path tests**

Test AI timeout, missing tool call, risk rejection, exchange rejection, synchronization failure, and watchdog expiration. Each test must assert a terminal persisted state.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./parity -run TestRunner -v`
Expected: FAIL because runner is absent.

- [ ] **Step 3: Implement serialized runner**

Use an agent-scoped mutex, persist every transition before invoking the next component, and ensure deferred cleanup always releases the mutex. The watchdog must not terminate an exchange request blindly; it marks the cycle failed and blocks duplicate execution until reconciliation completes.

- [ ] **Step 4: Run focused tests and commit**

Run: `go test ./parity -run TestRunner -v`
Expected: PASS.

Run: `git add parity/runner.go parity/runner_test.go && git commit -m "feat: assemble bounded parity cycle runner"`

### Task 8: Add authenticated API, dashboard data and shadow deployment

**Files:**
- Create: `nofx-source/api/handler_parity.go`
- Modify: `nofx-source/api/server.go`
- Create/modify: `nofx-source/web/` parity dashboard components following existing conventions
- Test: `nofx-source/api/handler_parity_test.go`

**Interfaces:**
- Produces endpoints for agent status, cycle detail, decisions, positions, orders, statistics and start/stop in shadow mode.
- Consumes runner status and persisted stores; enforces authenticated agent ownership.

- [ ] **Step 1: Add handler contract tests**

Assert status and cycle JSON fields, ownership checks, and that starting an agent defaults to shadow mode.

- [ ] **Step 2: Run API tests to verify failure**

Run: `go test ./api -run TestParity -v`
Expected: FAIL because routes and handlers are absent.

- [ ] **Step 3: Implement routes and response DTOs**

Use stable snake_case fields, return explicit `processing`, `failed`, and `completed` states, and expose `last_error_code`/`last_error_message` without raw secrets or full credential payloads.

- [ ] **Step 4: Run API and frontend checks**

Run: `go test ./api -run TestParity -v && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

Run: `git add api/handler_parity.go api/server.go api/handler_parity_test.go web && git commit -m "feat: expose parity agent shadow dashboard"`

### Task 9: Replay parity fixtures and install live safety gate

**Files:**
- Create: `nofx-source/parity/replay_test.go`
- Create: `nofx-source/docs/operations/VERGEX_PARITY_RUNBOOK.md`
- Modify: `nofx-source/config/` only for explicit shadow/live flag wiring

**Interfaces:**
- Produces a repeatable replay command and a live-mode confirmation guard.

- [ ] **Step 1: Add replay assertions**

Replay the captured prompt shape with synthetic market data and a hold tool call. Assert no secrets are emitted, the tool call is normalized, and the shadow executor produces zero outbound private exchange calls.

- [ ] **Step 2: Run the complete validation suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 3: Run static checks**

Run: `gofmt -w parity store api && go vet ./...`
Expected: no formatting changes pending and no vet errors.

- [ ] **Step 4: Document the live switch**

Require a runtime flag plus an operator confirmation record containing agent ID, exchange, account fingerprint, timestamp and confirmation text. Reject live startup if parity replay has not passed for the current build.

- [ ] **Step 5: Commit and stop before live activation**

Run: `git add parity docs/operations config && git commit -m "test: add parity replay and live safety gate"`

Do not enable live Bybit mode in this task. Present test output and request explicit confirmation separately.
