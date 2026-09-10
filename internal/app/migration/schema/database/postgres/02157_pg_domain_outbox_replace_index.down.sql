CREATE INDEX CONCURRENTLY idx_domain_event_outbox_unpublished
    ON domain_event_outbox (published_at, id);
