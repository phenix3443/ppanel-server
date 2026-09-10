ALTER TABLE `user_subscribe`
    DROP INDEX `idx_user_subscribe_plan_status_id`,
    ADD INDEX `idx_subscribe_id` (`subscribe_id`),
    ADD INDEX `idx_token` (`token`),
    ADD INDEX `idx_uuid` (`uuid`),
    ALGORITHM=INPLACE,
    LOCK=NONE;
