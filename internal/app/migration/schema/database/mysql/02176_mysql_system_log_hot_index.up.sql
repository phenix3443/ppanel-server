ALTER TABLE `system_logs`
    ADD INDEX `idx_system_log_type_object_id` (`type`, `object_id`, `id`),
    ALGORITHM=INPLACE,
    LOCK=NONE;
