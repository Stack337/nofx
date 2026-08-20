# Live Promotion Checklist

- [x] Focused Go tests for agent, agentcore, execution, provider, risk, and live API gates pass.
- [ ] Full `go test ./...` and `go vet ./...` pass in an environment with CGO/SQLite if required by legacy store tests.
- [ ] Frontend tests, TypeScript check, and production build pass.
- [ ] `docker compose --env-file .env.agent -f docker-compose.agent.yml config` passes on the VPS.
- [x] Shadow and Paper router paths make zero exchange mutation calls.
- [ ] Paper cycles use simulated fills and verify SL/TP behavior on the VPS.
- [x] Empty JSON, truncated JSON, unsupported function calls, provider HTTP 429/404/500, provider timeout, unknown order, duplicate failed retry, Bybit 34040, and position-mode mismatch tests pass.
- [x] Live remains unavailable without a configured passing health gate, one-way position mode, and `ENABLE LIVE <agent_id>`.
- [x] Kill switch can be disabled only with `DISABLE <agent_id>`.
- [ ] Bybit account health, instrument filters, balance, and permissions pass preflight on the VPS.
- [ ] Exactly one agent is confirmed with `ENABLE LIVE <agent_id>` on the VPS.
- [ ] First Live order, Bybit position, SL/TP, idempotency record, cycle, and audit event are reconciled manually.

No item may be skipped. A failed item returns the agent to Shadow and keeps Live disabled.
