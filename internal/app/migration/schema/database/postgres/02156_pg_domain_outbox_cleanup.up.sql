CREATE INDEX CONCURRENTLY idx_domain_event_outbox_published
    ON domain_event_outbox (published_at)
    WHERE published_at IS NOT NULL;
