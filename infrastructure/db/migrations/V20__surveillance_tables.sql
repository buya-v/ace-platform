-- V20: Market surveillance tables for trade pattern detection and alert generation
-- Part of T113: compliance-service surveillance capability

CREATE TABLE IF NOT EXISTS compliance.surveillance_rules (
    id VARCHAR(64) PRIMARY KEY,
    rule_type VARCHAR(50) NOT NULL, -- wash_trading, spoofing, price_manipulation, concentration, unusual_volume
    parameters JSONB NOT NULL,      -- thresholds, time windows, etc.
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS compliance.surveillance_alerts (
    id VARCHAR(64) PRIMARY KEY,
    rule_id VARCHAR(64) REFERENCES compliance.surveillance_rules(id),
    participant_id VARCHAR(64) NOT NULL,
    instrument_id VARCHAR(64),
    severity VARCHAR(20) NOT NULL, -- LOW, MEDIUM, HIGH, CRITICAL
    details JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'OPEN',
    detected_at TIMESTAMPTZ DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    resolver_id VARCHAR(64)
);

CREATE INDEX IF NOT EXISTS idx_surv_alerts_status ON compliance.surveillance_alerts(status);
CREATE INDEX IF NOT EXISTS idx_surv_alerts_participant ON compliance.surveillance_alerts(participant_id);
CREATE INDEX IF NOT EXISTS idx_surv_alerts_rule ON compliance.surveillance_alerts(rule_id);
CREATE INDEX IF NOT EXISTS idx_surv_alerts_detected ON compliance.surveillance_alerts(detected_at);

-- Default surveillance rules
INSERT INTO compliance.surveillance_rules (id, rule_type, parameters) VALUES
    ('wash-1', 'wash_trading', '{"time_window_seconds": 60, "min_trades": 2}'),
    ('spoof-1', 'spoofing', '{"cancel_window_seconds": 5, "min_order_size_pct": 5.0}'),
    ('concentration-1', 'concentration', '{"max_position_pct": 25.0}'),
    ('unusual-vol-1', 'unusual_volume', '{"std_dev_threshold": 3.0, "lookback_days": 30}')
ON CONFLICT (id) DO NOTHING;
