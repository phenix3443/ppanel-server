CREATE INDEX CONCURRENTLY idx_user_subscribe_lifecycle_expire
    ON user_subscribe (status, finished_at, expire_time);
