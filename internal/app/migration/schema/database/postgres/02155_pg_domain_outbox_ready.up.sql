CREATE INDEX CONCURRENTLY idx_domain_event_outbox_ready
    ON domain_event_outbox (id)
    WHERE published_at IS NULL;
