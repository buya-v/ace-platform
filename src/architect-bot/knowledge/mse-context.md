# Mongolia Stock Exchange (MSE) Context

## MSE Overview

The Mongolia Stock Exchange (MSE) was established in 1991 as part of Mongolia's transition to a market economy. It is the primary securities exchange in Mongolia, located in Ulaanbaatar.

- **Listed companies**: ~200 (equities, bonds, ETFs)
- **Currency**: Mongolian Tugrik (MNT)
- **Settlement cycle**: T+2
- **Daily trading volume**: ~2,000 trades/day (relatively low by global standards)
- **Market cap**: ~$3-4 billion USD (small frontier market)
- **Trading hours**: 10:00-13:00 UB time (UTC+8), with pre-open and closing auction phases

## Current Technology: MillenniumIT

MSE currently runs on MillenniumIT (MIT) technology, provided by London Stock Exchange Technology (LSET). MIT is deployed at 40+ exchanges globally.

**MIT characteristics at MSE:**
- Proprietary C++ trading engine with custom OUCH/ITCH-derived protocols
- Single-venue deployment (one instance per exchange)
- Annual licensing fees payable to LSEG
- Vendor-dependent upgrades and customization
- Limited AI or automation capabilities
- Requires specialized MIT expertise for maintenance

## Regulatory Environment

### FRC (Financial Regulatory Commission)
Mongolia's unified financial regulator overseeing securities, insurance, and non-bank financial institutions. Key functions:
- Market surveillance and oversight
- Licensing of securities firms and brokers
- Approval of new listings
- Enforcement of trading rules and disclosure requirements
- Regular reporting requirements for exchange operators

### MCSD (Mongolian Central Securities Depository)
The central securities depository responsible for:
- Securities ownership registration
- Custody account management
- Settlement of securities transactions (DVP)
- Corporate action processing (dividend distribution, splits)
- Investor account management

GarudaX integrates with MCSD through the CSD module (CustodyAccount, CustodyBalance, CSDTransfer types with DVP and FOP transfer support).

## MSE Challenges

1. **Aging technology**: The MIT system, while reliable, is based on older architecture that limits innovation
2. **High licensing costs**: Annual fees to LSEG represent a significant expense for a small exchange
3. **Vendor lock-in**: All upgrades, customizations, and new features depend on MIT/LSEG roadmap and pricing
4. **Limited customization**: Changes to trading rules, new instrument types, or market structure modifications require vendor engagement
5. **No AI integration**: The current system has no capability for AI-powered operations, analytics, or natural language administration
6. **Scaling constraints**: While MSE's current volume is low, the proprietary architecture limits future growth options
7. **Talent dependency**: Maintaining MIT systems requires specialized knowledge that is scarce in Mongolia

## GarudaX Value Proposition for MSE

### Cost Reduction
- Eliminate annual MIT licensing fees
- Use standard open-source infrastructure (PostgreSQL, Kafka, Docker)
- Local team can maintain and extend the system without vendor dependency

### Technology Modernization
- Modern Go microservices architecture vs monolithic C++
- React-based operator dashboards vs legacy desktop clients
- Standard FIX 4.4 protocol for broker connectivity (no proprietary protocol adapters needed)
- PostgreSQL/TimescaleDB for market data (standard SQL analytics vs proprietary data stores)

### AI-Native Operations
- Natural language admin operations through admin-bot
- 9 MCP tools for programmatic exchange management
- AI-powered surveillance assistance
- Automated reporting and anomaly detection

### Sovereignty and Control
- MSE owns the source code and infrastructure
- No dependency on foreign vendor roadmap or pricing decisions
- Local development team can add features, modify trading rules, and respond to FRC requirements without vendor mediation
- Kubernetes deployment can run on local infrastructure or any cloud provider

### Multi-Tenant Capability
- GarudaX can host MSE equities alongside other venues (e.g., a commodity exchange) on the same platform
- Shared infrastructure reduces total cost of ownership
- Each venue maintains complete isolation (separate tenant context, trading rules, participants)

### Regulatory Compliance
- FRC reporting module (FRCReport type with configurable report types)
- Full audit trail (AuditEntry with entity tracking, actor ID, timestamps)
- 12-pattern surveillance engine with investigation workflow
- Maker-checker (four-eyes) approval for critical changes

## Why MSE Should Choose GarudaX

1. **Proven core**: 47% functional depth vs MIT and growing at ~3% per sprint. Core trading (matching, settlement, clearing) is SUBSTANTIAL or higher.
2. **Right-sized**: MSE processes ~2,000 trades/day. GarudaX's architecture handles this with headroom. MIT's partition-based scaling is designed for 100K+ msg/sec exchanges — overkill for MSE.
3. **Future-proof**: Multi-tenant architecture means MSE can host additional venues. AI integration positions MSE as a technology leader among frontier market exchanges.
4. **Cost trajectory**: MIT licensing costs are recurring. GarudaX development cost is one-time with ongoing maintenance at a fraction of licensing fees.
5. **FIX 4.4 compatibility**: Existing brokers using FIX connectivity can connect to GarudaX with minimal changes. No need for proprietary protocol migration.
6. **Local control**: FRC regulatory changes can be implemented directly without waiting for vendor cycles. Trading rule modifications are code changes, not vendor negotiations.
