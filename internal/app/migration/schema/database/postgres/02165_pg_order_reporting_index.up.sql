CREATE INDEX CONCURRENTLY idx_order_status_created_at
    ON "order" (status, created_at);
