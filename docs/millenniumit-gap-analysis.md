# MillenniumIT vs GarudaX — Gap Analysis

**Date:** 2026-04-24
**Source:** MillenniumIT platform documents from /home/vcp/MilleniumIT/
**Purpose:** Identify feature gaps for MSE flagship tenant migration

## Coverage Summary

| Category | Features | Covered | Partial | Missing |
|----------|---------|---------|---------|---------|
| Trading Engine | 10 | 0 | 0 | 10 |
| Market Management | 6 | 0 | 2 | 4 |
| Instrument Management | 7 | 1 | 1 | 5 |
| Trading Parameters | 4 | 0 | 1 | 3 |
| Post-Trade | 6 | 1 | 0 | 5 |
| Corporate Actions | 1 | 1 | 0 | 0 |
| Surveillance | 5 | 0 | 1 | 4 |
| Investigation | 5 | 0 | 0 | 5 |
| Market Replay | 3 | 0 | 0 | 3 |
| User & Access Control | 8 | 0 | 1 | 7 |
| Reference Data Mgmt | 3 | 0 | 1 | 2 |
| Service Desk | 4 | 0 | 1 | 3 |
| CSD | 3 | 0 | 1 | 2 |
| Connectivity | 4 | 0 | 0 | 4 |
| Reporting | 2 | 1 | 0 | 1 |
| **TOTAL** | **71** | **4 (6%)** | **9 (13%)** | **58 (81%)** |

## Priority 1 — Exchange Cannot Operate Without These

1. Market/Segment hierarchy + configurable trading cycles
2. Circuit breakers (static + dynamic price bands)
3. Participant/Firm/Node hierarchy (broker-dealer model)
4. Granular role-based privileges (4-tier)
5. Start/End Day workflow
6. Iceberg orders + IOC/FOK time-in-force
7. Mass cancel (by firm/user/instrument)
8. Self-trade prevention

## Priority 2 — Required for Production

9. Order throttling (per-user rate limiting)
10. Tiered tick tables
11. Force logout + lock/unlock + user suspension
12. Reference price management
13. Dual authorization (maker-checker)
14. Trade correction (bust/correct/reinstate)
15. Announcements system
16. Activity log / audit trail UI
17. FIX protocol gateway

## Priority 3 — Full MSE Parity

18. Real-time surveillance engine
19. Market data dissemination (FAST/MITCH)
20. Off-book trade reporting
21. Service desk operations front-end
22. Instrument groups
23. Quote entry / mass quoting
24. Drop copy service
25. Instrument deletion lifecycle
26. Reference data bulk upload

## Priority 4 — Advanced

27. Investigation manager
28. Market replay
29. Fixed income support
30. Multi-leg instruments
31. CSD integration
32. Give-up/give-in
33. RFQ system
34. Short selling controls
35. Native binary protocol

## Build Approach

Phase 1 (P1, ~8 sprints): Exchange operations foundation
Phase 2 (P2, ~6 sprints): Production hardening
Phase 3 (P3-4, ~10+ sprints): Surveillance + advanced features
