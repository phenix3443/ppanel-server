ALTER TABLE `system_logs`
    DROP INDEX `idx_system_log_type_object_id`,
    ALGORITHM=INPLACE,
    LOCK=NONE;
