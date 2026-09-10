ALTER TABLE `traffic_log`
    DROP INDEX `idx_traffic_server_time`,
    DROP INDEX `idx_traffic_user_sub_time`,
    ADD INDEX `idx_server_id` (`server_id`),
    ADD INDEX `idx_user_id` (`user_id`),
    ALGORITHM=INPLACE,
    LOCK=NONE;
