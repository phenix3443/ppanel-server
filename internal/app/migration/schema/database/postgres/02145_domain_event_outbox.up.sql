-- Generic domain-event outbox (ADR-001 step-6 preparation); see the MySQL
-- variant for the design notes.
CREATE TABLE domain_event_outbox (
    id BIGSERIAL PRIMARY KEY,
    topic VARCHAR(64) NOT NULL,
    event_key VARCHAR(191) NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    published_at TIMESTAMPTZ NULL DEFAULT NULL
);
CREATE INDEX idx_domain_event_outbox_unpublished ON domain_event_outbox (published_at, id);

CREATE TABLE subscription_user_serial (
    user_id BIGINT NOT NULL,
    PRIMARY KEY (user_id)
);
