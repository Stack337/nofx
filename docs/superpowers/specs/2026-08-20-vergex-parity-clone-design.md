# VergeX Parity Trading Clone Design

**Status:** design baseline for implementation

**Goal:** build a separately deployable trading application that reproduces the observed VergeX cycle, AI/tool protocol, risk gates, Bybit execution, persistence, API, and dashboard behavior, while keeping live trading disabled until parity verification and an explicit live-switch confirmation.

## Scope and non-goals

The clone covers one-way Bybit USDT perpetual trading first. It includes account state, candidate selection, market context, AI analysis, mandatory trading tool calls, risk validation, market orders, conditional stop-loss/take-profit orders, order/fill synchronization, decision history, statistics, and the agent dashboard/API.

The first release does not copy unrelated NOFX exchange integrations, billing, public leaderboards, or undocumented VergeX internals. Those remain extension points after Bybit parity is proven.

## Observed VergeX contract

The active agent sends an OpenAI-compatible request with two messages: a system prompt and a large market-analysis user prompt. The initial request is streaming and contains no tools. The follow-up request contains ten trading tools and requires a tool call. The adapter must preserve streaming semantics and normalize upstream SSE comments and duplicate `[DONE]` sentinels.

The observed cycle is:

1. load account, open positions, recent orders, and strategy configuration;
2. select the candidate universe from AI500/signals and active positions;
3. fetch candles, mark/last prices, open interest, funding, positioning and liquidation data;
4. build the system and user prompts;
5. request text-only market analysis;
6. request a mandatory trading function call, retrying the tool round when the model returns text or an incomplete stream;
7. validate symbol, side, size, leverage, stop and target against hard risk rules;
8. execute closes before opens, then synchronize exchange state;
9. save the cycle, raw prompts/responses, action results, equity snapshot and errors;
10. expose the updated state to the dashboard.

The clone must treat a missing function call, missing completed event, malformed JSON, upstream 4xx/5xx, timeout, and rate limit as explicit terminal outcomes for that cycle. It must never leave an in-memory `Processing` state without a persisted timeout/error transition.

## Components

### Cycle orchestrator

Owns a per-agent mutex and a persisted cycle state machine:

`scheduled -> collecting_context -> analysis_round -> tool_round -> validating -> executing -> synchronizing -> completed`

Any failure transitions to `failed` with a machine-readable error code and a human-readable message. A watchdog marks a cycle failed after the configured request deadline and permits the next scheduled cycle.

### AI gateway adapter

Provides an internal interface independent of provider format:

```text
Analyze(systemPrompt, userPrompt) -> AnalysisText
RequestDecision(messages, tools, requiredTool) -> ToolCall
```

The adapter supports OpenAI Chat Completions and Responses-shaped upstreams. It filters SSE comments, accepts one terminal `[DONE]`, validates event order, repairs a missing final `output` only when the function-call argument event is already complete, and rejects ambiguous streams. For DeepSeek/OpenRouter tool requests it removes unsupported `parallel_tool_calls` immediately before provider routing and uses an allowlisted provider order; it never relies on client IP rotation to solve upstream shared-pool limits.

### Market context builder

Fetches active positions first, then candidates. It keeps only candidates with valid market data, preserves the exact symbol normalization used by the execution layer, and records the snapshot timestamp and data-source status. Forming candles are labeled as live snapshots and are not silently treated as closed candles.

### Risk engine

Validates every open action before exchange calls. Required checks are positive size/leverage/SL/TP, side-consistent SL/TP ordering, minimum order value, per-symbol position cap, maximum leverage, and minimum risk/reward. Close and reduce actions remain available when price data is degraded; new exposure is blocked on stale or divergent price data.

### Bybit execution and synchronization

Uses Bybit V5 linear orders in one-way mode with `positionIdx=0`. Opens cancel stale ordinary and conditional orders, set leverage, place a market order, then create reduce-only conditional SL and TP orders. Closes use reduce-only market orders. Every order is reconciled from exchange history/trades; local state is updated from actual fill quantity, average price and fees, not requested values. “Already cancelled/not modified” responses are idempotent success when the desired final state is already present.

### Persistence

Use separate records for agents, cycles, prompts, tool calls, decisions, orders, fills, positions, equity snapshots, and provider errors. Store a correlation ID on every record. Secrets are encrypted at rest and are never included in prompts, logs, exports or this repository.

## API and UI parity

The clone exposes authenticated endpoints for agent list/configuration, start/stop, status, account, positions, open orders, decisions, latest decisions, statistics, equity history and cycle detail. Responses use stable snake_case fields matching the observed dashboard contract. The UI shows the current cycle state, last error, model alias, balance/equity, positions, orders, recent decisions and a clear stale-processing indicator.

The UI must distinguish:

- AI call failure;
- AI decision without a function call;
- exchange order rejection;
- synchronization failure;
- risk-blocked action;
- an actually active cycle.

## Safety gates

The clone starts in shadow mode. Shadow mode performs all context building, prompt generation, AI calls, validation and simulated execution, but sends no private Bybit order request. A parity harness replays captured prompt fixtures and synthetic tool calls, compares normalized decisions and state transitions, and runs exchange-contract tests against a mock Bybit server. Live mode can be enabled only after these tests pass and the user explicitly confirms the live switch.

## Parity acceptance criteria

Parity is considered sufficient for the first release when:

1. the same context fixture produces equivalent system/user prompt sections and candidate ordering;
2. a valid tool stream produces the same normalized action and arguments;
3. missing/duplicate SSE terminals and incomplete Responses output reach the same explicit error or repair path;
4. risk fixtures accept and reject the same actions;
5. Bybit mock tests verify `positionIdx=0`, reduce-only closes, conditional SL/TP, idempotent cancellation and fill reconciliation;
6. a cycle cannot remain indefinitely in `Processing`;
7. shadow-mode replay produces no live exchange request;
8. all secrets are absent from logs and persisted diagnostic artifacts.

## Implementation order

Implement in this order: domain/state machine, persistence, deterministic risk engine, AI adapter and stream tests, market context fixtures, Bybit mock/execution, shadow cycle runner, API/UI, parity replay, then explicit live-mode gate. Do not modify the existing VergeX services while building the clone.
