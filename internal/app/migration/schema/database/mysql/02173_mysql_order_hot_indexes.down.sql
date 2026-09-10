ALTER TABLE `order`
    DROP INDEX `idx_order_status_id`,
    DROP INDEX `idx_order_status_created_at`,
    DROP INDEX `idx_order_user_status_id`,
    DROP INDEX `idx_order_subscribe_status_id`,
    ALGORITHM=INPLACE,
    LOCK=NONE;
