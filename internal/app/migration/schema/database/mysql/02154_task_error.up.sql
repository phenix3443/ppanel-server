CREATE TABLE `task_error` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `task_id` bigint NOT NULL,
    `position` bigint unsigned NOT NULL,
    `target` varchar(320) NOT NULL DEFAULT '',
    `error` text,
    `occurred_at` bigint NOT NULL DEFAULT 0,
    `created_at` datetime(3) DEFAULT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_task_error_position` (`task_id`, `position`),
    KEY `idx_task_error_task_id` (`task_id`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
