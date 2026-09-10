CREATE INDEX CONCURRENTLY idx_order_status_id
    ON "order" (status, id);
