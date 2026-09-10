-- Generic domain-event outbox (ADR-001 step-6 preparation): a module appends
-- the event in the same transaction as its domain write; the in-process
-- dispatcher delivers it to subscribing modules (idempotent via the inbox)
-- and marks it published.  Swapping the dispatcher for a message broker is
-- a driver change.  Also adds the subscription domain's per-user serial
-- lock row, replacing the fulfillment stage's cross-domain user-row lock.
CREATE TABLE `domain_event_outbox` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `topic` VARCHAR(64) NOT NULL COMMENT 'Event topic, e.g. identity.user_registered',
    `event_key` VARCHAR(191) NOT NULL COMMENT 'Business key of the event',
    `payload` TEXT NOT NULL,
    `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `published_at` DATETIME(3) NULL DEFAULT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_unpublished` (`published_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE `subscription_user_serial` (
    `user_id` BIGINT NOT NULL,
    PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
