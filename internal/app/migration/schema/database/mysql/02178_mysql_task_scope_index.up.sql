ALTER TABLE `task`
    ADD INDEX `idx_task_scope_created_id` (`scope_type`, `created_at` DESC, `id` DESC),
    ALGORITHM=DEFAULT,
    LOCK=NONE;
