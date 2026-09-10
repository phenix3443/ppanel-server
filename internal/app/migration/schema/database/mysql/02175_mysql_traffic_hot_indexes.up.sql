ALTER TABLE `traffic_log`
    ADD INDEX `idx_traffic_server_time` (`server_id`, `timestamp`),
    ADD INDEX `idx_traffic_user_sub_time` (`user_id`, `subscribe_id`, `timestamp`),
    DROP INDEX `idx_server_id`,
    DROP INDEX `idx_user_id`,
    ALGORITHM=INPLACE,
    LOCK=NONE;
