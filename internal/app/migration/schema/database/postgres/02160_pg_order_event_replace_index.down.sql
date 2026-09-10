CREATE INDEX CONCURRENTLY idx_order_event_published_at_id
    ON order_event (published_at, id);
