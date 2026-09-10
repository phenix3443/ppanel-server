CREATE INDEX CONCURRENTLY idx_order_event_published_created
    ON order_event (created_at, id)
    WHERE published_at IS NOT NULL;
