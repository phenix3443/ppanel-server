CREATE INDEX CONCURRENTLY idx_order_event_ready
    ON order_event (id)
    WHERE published_at IS NULL;
