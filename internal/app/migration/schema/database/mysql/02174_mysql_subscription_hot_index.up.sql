ALTER TABLE `user_subscribe`
    ADD INDEX `idx_user_subscribe_plan_status_id` (`subscribe_id`, `status`, `id`),
    DROP INDEX `idx_subscribe_id`,
    DROP INDEX `idx_token`,
    DROP INDEX `idx_uuid`,
    ALGORITHM=INPLACE,
    LOCK=NONE;
