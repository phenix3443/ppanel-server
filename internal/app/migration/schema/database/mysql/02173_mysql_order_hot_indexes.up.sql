ALTER TABLE `order`
    ADD INDEX `idx_order_status_id` (`status`, `id`),
    ADD INDEX `idx_order_status_created_at` (`status`, `created_at`),
    ADD INDEX `idx_order_user_status_id` (`user_id`, `status`, `id`),
    ADD INDEX `idx_order_subscribe_status_id` (`subscribe_id`, `status`, `id`),
    ALGORITHM=INPLACE,
    LOCK=NONE;
