ALTER TABLE `task`
    DROP INDEX `idx_task_scope_created_id`,
    ALGORITHM=DEFAULT,
    LOCK=NONE;
