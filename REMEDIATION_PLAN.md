
---

## R0-Extension — additional money-float paths (added 2026-06-26, from R006 audit)

The R006 money-path audit confirmed R003/R004 scope is clean but found **four more services with residual float64 money math** beyond the original securities focus. Because the project is production-bound, these are P0 follow-ups (financial correctness) and should run **before** R1/R2. Tasks R018–R022 in `tasks-remediation.json` (status `deferred`).

| ID | Service | Residual money-float path |
|----|---------|---------------------------|
| R018 | corporate-actions (+ securities CA handler) | Entire engine on float: dividend cash, rights subscription cost, split/ex-rights price, float `round2`. Not yet wired (no `main.go`) — migrate before it goes live. Includes `handlers_corporate_actions.go:233` dividend float + `EntitlementValue`. |
| R019 | compliance-service (MCSD/FRC) | Settlement amount, cash entitlement, settlement-fail penalties via float `round2`. |
| R020 | gateway | Trading **fee** calculation + **pre-trade risk** order-value/limit checks in float64. |
| R021 | fix-gateway | FIX `Price`/`StopPrice` carried as float64 through the protocol mapper. |
| R022 | (audit) | Re-run money-path audit platform-wide to confirm zero residual float money math. |

R006 also **fixed a margin-engine haircut truncation bug** in passing (`int64(haircut*10000)` → `decimal.NewFromFloat`), already merged.

**Sequencing:** R018–R021 are independent (parallelizable, all blockedBy R002 only) → R022 audit gate. Run this cluster next, then R1/R2/R3/R4.
