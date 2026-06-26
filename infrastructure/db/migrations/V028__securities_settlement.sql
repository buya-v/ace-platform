-- V28: Securities settlement and CSD integration tables

-- Settlement obligations (novated trades pending settlement)
CREATE TABLE securities.settlement_obligations (
    obligation_id       VARCHAR(64) PRIMARY KEY,
    trade_id            VARCHAR(64) NOT NULL,
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    participant_id      VARCHAR(64) NOT NULL,
    counterparty_id     VARCHAR(64) NOT NULL DEFAULT 'GARUDAX-CCP',
    side                VARCHAR(4) NOT NULL CHECK (side IN ('BUY', 'SELL')),
    quantity            BIGINT NOT NULL,
    price               DECIMAL(18,4) NOT NULL,
    settlement_value    DECIMAL(18,4) NOT NULL,
    accrued_interest    DECIMAL(18,4) NOT NULL DEFAULT 0,
    settlement_date     DATE NOT NULL,
    status              VARCHAR(15) NOT NULL DEFAULT 'PENDING' CHECK (status IN (
        'PENDING', 'AFFIRMED', 'NETTED', 'INSTRUCTED', 'SETTLING', 'SETTLED', 'FAILED'
    )),
    netting_run_id      VARCHAR(64),
    csd_instruction_id  VARCHAR(64),
    fail_reason         TEXT,
    penalty_accrued     DECIMAL(18,4) NOT NULL DEFAULT 0,
    buyer_affirmed      BOOLEAN NOT NULL DEFAULT FALSE,
    seller_affirmed     BOOLEAN NOT NULL DEFAULT FALSE,
    affirmed_at         TIMESTAMPTZ,
    netted_at           TIMESTAMPTZ,
    instructed_at       TIMESTAMPTZ,
    settled_at          TIMESTAMPTZ,
    failed_at           TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sec_obligations_trade ON securities.settlement_obligations(trade_id);
CREATE INDEX idx_sec_obligations_instrument ON securities.settlement_obligations(instrument_id);
CREATE INDEX idx_sec_obligations_participant ON securities.settlement_obligations(participant_id);
CREATE INDEX idx_sec_obligations_settlement_date ON securities.settlement_obligations(settlement_date);
CREATE INDEX idx_sec_obligations_status ON securities.settlement_obligations(status);
CREATE INDEX idx_sec_obligations_netting_run ON securities.settlement_obligations(netting_run_id) WHERE netting_run_id IS NOT NULL;

-- Netting results for securities
CREATE TABLE securities.netting_results (
    id                  VARCHAR(64) PRIMARY KEY,
    run_id              VARCHAR(64) NOT NULL,
    participant_id      VARCHAR(64) NOT NULL,
    instrument_id       VARCHAR(64) NOT NULL,
    settlement_date     DATE NOT NULL,
    net_qty             BIGINT NOT NULL DEFAULT 0,
    net_value           DECIMAL(18,4) NOT NULL DEFAULT 0,
    net_accrued_interest DECIMAL(18,4) NOT NULL DEFAULT 0,
    gross_buy_qty       BIGINT NOT NULL DEFAULT 0,
    gross_sell_qty      BIGINT NOT NULL DEFAULT 0,
    obligations_count   INT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sec_netting_run ON securities.netting_results(run_id);
CREATE INDEX idx_sec_netting_participant ON securities.netting_results(participant_id);
CREATE INDEX idx_sec_netting_settlement_date ON securities.netting_results(settlement_date);

-- CSD custody accounts
CREATE TABLE securities.csd_accounts (
    account_id          VARCHAR(64) PRIMARY KEY,
    participant_id      VARCHAR(64) NOT NULL,
    csd_account_ref     VARCHAR(30) NOT NULL UNIQUE,
    account_type        VARCHAR(25) NOT NULL DEFAULT 'PROPRIETARY' CHECK (account_type IN (
        'PROPRIETARY', 'CLIENT_SEGREGATED', 'COLLATERAL'
    )),
    status              VARCHAR(10) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN (
        'ACTIVE', 'FROZEN', 'CLOSED'
    )),
    opened_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at           TIMESTAMPTZ
);

CREATE INDEX idx_csd_accounts_participant ON securities.csd_accounts(participant_id);
CREATE INDEX idx_csd_accounts_status ON securities.csd_accounts(status);

-- CSD holdings (balances per account per instrument)
CREATE TABLE securities.csd_balances (
    account_id          VARCHAR(64) NOT NULL REFERENCES securities.csd_accounts(account_id),
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    total_qty           BIGINT NOT NULL DEFAULT 0,
    available_qty       BIGINT NOT NULL DEFAULT 0,
    pledged_qty         BIGINT NOT NULL DEFAULT 0,
    pending_in_qty      BIGINT NOT NULL DEFAULT 0,
    pending_out_qty     BIGINT NOT NULL DEFAULT 0,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (account_id, instrument_id),
    CONSTRAINT chk_available CHECK (available_qty >= 0),
    CONSTRAINT chk_total CHECK (total_qty >= 0),
    CONSTRAINT chk_balance CHECK (available_qty = total_qty - pledged_qty - pending_out_qty)
);

CREATE INDEX idx_csd_balances_instrument ON securities.csd_balances(instrument_id);

-- CSD transfers (FoP and DvP)
CREATE TABLE securities.csd_transfers (
    transfer_id         VARCHAR(64) PRIMARY KEY,
    settlement_obligation_id VARCHAR(64),
    from_account_id     VARCHAR(64) NOT NULL REFERENCES securities.csd_accounts(account_id),
    to_account_id       VARCHAR(64) NOT NULL REFERENCES securities.csd_accounts(account_id),
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    quantity            BIGINT NOT NULL,
    transfer_type       VARCHAR(5) NOT NULL CHECK (transfer_type IN ('FOP', 'DVP')),
    settlement_value    DECIMAL(18,4) DEFAULT 0,
    reason              VARCHAR(50),
    status              VARCHAR(10) NOT NULL DEFAULT 'PENDING' CHECK (status IN (
        'PENDING', 'MATCHED', 'COMPLETED', 'FAILED', 'REJECTED'
    )),
    fail_reason         TEXT,
    instructed_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    matched_at          TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ
);

CREATE INDEX idx_csd_transfers_obligation ON securities.csd_transfers(settlement_obligation_id) WHERE settlement_obligation_id IS NOT NULL;
CREATE INDEX idx_csd_transfers_from ON securities.csd_transfers(from_account_id);
CREATE INDEX idx_csd_transfers_to ON securities.csd_transfers(to_account_id);
CREATE INDEX idx_csd_transfers_instrument ON securities.csd_transfers(instrument_id);
CREATE INDEX idx_csd_transfers_status ON securities.csd_transfers(status);

-- Settlement fail penalties
CREATE TABLE securities.settlement_penalties (
    penalty_id          VARCHAR(64) PRIMARY KEY,
    obligation_id       VARCHAR(64) NOT NULL REFERENCES securities.settlement_obligations(obligation_id),
    failing_participant_id VARCHAR(64) NOT NULL,
    penalty_date        DATE NOT NULL,
    failed_value        DECIMAL(18,4) NOT NULL,
    penalty_rate_bps    DECIMAL(5,2) NOT NULL,
    penalty_amount      DECIMAL(18,4) NOT NULL,
    status              VARCHAR(10) NOT NULL DEFAULT 'ACCRUED' CHECK (status IN (
        'ACCRUED', 'INVOICED', 'PAID', 'WAIVED'
    )),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sec_penalties_obligation ON securities.settlement_penalties(obligation_id);
CREATE INDEX idx_sec_penalties_participant ON securities.settlement_penalties(failing_participant_id);
CREATE INDEX idx_sec_penalties_status ON securities.settlement_penalties(status);

-- Buy-in records
CREATE TABLE securities.buy_ins (
    buy_in_id           VARCHAR(64) PRIMARY KEY,
    obligation_id       VARCHAR(64) NOT NULL REFERENCES securities.settlement_obligations(obligation_id),
    initiated_by        VARCHAR(64) NOT NULL,
    failing_participant_id VARCHAR(64) NOT NULL,
    instrument_id       VARCHAR(64) NOT NULL,
    original_qty        BIGINT NOT NULL,
    original_price      DECIMAL(18,4) NOT NULL,
    buy_in_qty          BIGINT,
    buy_in_price        DECIMAL(18,4),
    cost_difference     DECIMAL(18,4),
    status              VARCHAR(15) NOT NULL DEFAULT 'NOTIFIED' CHECK (status IN (
        'NOTIFIED', 'GRACE_PERIOD', 'EXECUTING', 'COMPLETED', 'CANCELLED'
    )),
    notification_date   DATE NOT NULL,
    grace_deadline      DATE NOT NULL,
    execution_date      DATE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ
);

CREATE INDEX idx_buy_ins_obligation ON securities.buy_ins(obligation_id);
CREATE INDEX idx_buy_ins_failing ON securities.buy_ins(failing_participant_id);
CREATE INDEX idx_buy_ins_status ON securities.buy_ins(status);

-- Corporate actions
CREATE TABLE securities.corporate_actions (
    action_id           VARCHAR(64) PRIMARY KEY,
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    action_type         VARCHAR(20) NOT NULL CHECK (action_type IN (
        'DIVIDEND', 'STOCK_DIVIDEND', 'STOCK_SPLIT', 'REVERSE_SPLIT',
        'RIGHTS_ISSUE', 'MERGER', 'TENDER_OFFER', 'SPIN_OFF'
    )),
    announcement_date   DATE NOT NULL,
    ex_date             DATE NOT NULL,
    record_date         DATE NOT NULL,
    payment_date        DATE,
    status              VARCHAR(25) NOT NULL DEFAULT 'ANNOUNCED' CHECK (status IN (
        'ANNOUNCED', 'EX_DATE_PASSED', 'RECORD_DATE_PASSED', 'PROCESSED', 'CANCELLED'
    )),

    -- Cash dividend fields
    dividend_per_share  DECIMAL(18,4) DEFAULT 0,
    dividend_currency   CHAR(3) DEFAULT 'MNT',

    -- Split fields
    split_ratio_from    INT DEFAULT 1,
    split_ratio_to      INT DEFAULT 1,

    -- Rights issue fields
    rights_ratio        DECIMAL(8,4) DEFAULT 0,
    exercise_price      DECIMAL(18,4) DEFAULT 0,
    rights_instrument_id VARCHAR(64),

    -- Merger fields
    target_instrument_id VARCHAR(64),
    conversion_ratio    DECIMAL(8,4) DEFAULT 0,
    cash_component      DECIMAL(18,4) DEFAULT 0,

    description         TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_corp_actions_instrument ON securities.corporate_actions(instrument_id);
CREATE INDEX idx_corp_actions_type ON securities.corporate_actions(action_type);
CREATE INDEX idx_corp_actions_ex_date ON securities.corporate_actions(ex_date);
CREATE INDEX idx_corp_actions_record_date ON securities.corporate_actions(record_date);
CREATE INDEX idx_corp_actions_status ON securities.corporate_actions(status);

-- Corporate action entitlements (per participant)
CREATE TABLE securities.corporate_action_entitlements (
    entitlement_id      VARCHAR(64) PRIMARY KEY,
    action_id           VARCHAR(64) NOT NULL REFERENCES securities.corporate_actions(action_id),
    participant_id      VARCHAR(64) NOT NULL,
    csd_account_id      VARCHAR(64) NOT NULL,
    holding_qty         BIGINT NOT NULL,
    entitlement_type    VARCHAR(10) NOT NULL CHECK (entitlement_type IN ('CASH', 'SHARES', 'RIGHTS')),
    cash_amount         DECIMAL(18,4) DEFAULT 0,
    shares_qty          BIGINT DEFAULT 0,
    status              VARCHAR(15) NOT NULL DEFAULT 'PENDING' CHECK (status IN (
        'PENDING', 'PROCESSED', 'PAID', 'FAILED'
    )),
    payment_date        DATE,
    processed_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_entitlements_action ON securities.corporate_action_entitlements(action_id);
CREATE INDEX idx_entitlements_participant ON securities.corporate_action_entitlements(participant_id);
CREATE INDEX idx_entitlements_status ON securities.corporate_action_entitlements(status);

-- Grant access
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'garudax_clearing_svc') THEN
        GRANT SELECT, INSERT, UPDATE ON securities.settlement_obligations TO garudax_clearing_svc;
        GRANT SELECT, INSERT ON securities.netting_results TO garudax_clearing_svc;
        GRANT SELECT, INSERT, UPDATE ON securities.csd_balances TO garudax_clearing_svc;
        GRANT SELECT, INSERT, UPDATE ON securities.csd_transfers TO garudax_clearing_svc;
        GRANT SELECT, INSERT ON securities.settlement_penalties TO garudax_clearing_svc;
        GRANT SELECT, INSERT, UPDATE ON securities.buy_ins TO garudax_clearing_svc;
        GRANT SELECT ON securities.corporate_actions TO garudax_clearing_svc;
        GRANT SELECT, INSERT, UPDATE ON securities.corporate_action_entitlements TO garudax_clearing_svc;
        GRANT SELECT ON securities.csd_accounts TO garudax_clearing_svc;
    END IF;
END $$;
