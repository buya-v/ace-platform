CREATE SCHEMA IF NOT EXISTS tickets;

CREATE TABLE tickets.tickets (
    id VARCHAR(64) PRIMARY KEY,
    title VARCHAR(500) NOT NULL,
    description TEXT NOT NULL,
    category VARCHAR(30) NOT NULL, -- bug_report, customization, support, feature_request
    priority VARCHAR(20) NOT NULL DEFAULT 'medium', -- low, medium, high, critical
    status VARCHAR(20) NOT NULL DEFAULT 'open', -- open, in_progress, resolved, closed
    reporter_id VARCHAR(64) NOT NULL,
    assignee_id VARCHAR(64),
    tags TEXT[],
    metadata JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);
CREATE INDEX idx_tickets_status ON tickets.tickets(status);
CREATE INDEX idx_tickets_reporter ON tickets.tickets(reporter_id);
CREATE INDEX idx_tickets_category ON tickets.tickets(category);
CREATE INDEX idx_tickets_priority ON tickets.tickets(priority);

CREATE TABLE tickets.comments (
    id VARCHAR(64) PRIMARY KEY,
    ticket_id VARCHAR(64) REFERENCES tickets.tickets(id),
    author_id VARCHAR(64) NOT NULL,
    body TEXT NOT NULL,
    is_bot BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_comments_ticket ON tickets.comments(ticket_id);
