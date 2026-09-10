CREATE INDEX CONCURRENTLY idx_user_subscribe_active_expire
    ON user_subscribe (expire_time, id)
    WHERE status IN (0, 1) AND finished_at IS NULL;
