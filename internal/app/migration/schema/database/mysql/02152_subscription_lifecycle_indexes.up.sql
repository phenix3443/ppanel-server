CREATE INDEX `idx_user_subscribe_lifecycle_expire`
    ON `user_subscribe` (`status`, `finished_at`, `expire_time`);

CREATE INDEX `idx_user_subscribe_lifecycle_traffic`
    ON `user_subscribe` (`status`, `traffic`);

CREATE INDEX `idx_user_auth_methods_type_user`
    ON `user_auth_methods` (`auth_type`, `user_id`);
