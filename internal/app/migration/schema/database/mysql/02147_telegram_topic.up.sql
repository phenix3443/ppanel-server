-- The Telegram bot mirrors its administrator surface into a forum-enabled
-- supergroup: one feed topic for operational notifications, one topic per
-- website ticket, and one live-chat topic per bound panel user. This table
-- is the only mapping between a forum topic (message_thread_id) and what it
-- carries; the two unique keys are the cross-talk guarantee — a conversation
-- has at most one topic, and a topic carries at most one conversation.
CREATE TABLE `telegram_topic` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `chat_id` BIGINT NOT NULL DEFAULT 0 COMMENT 'Admin group chat id',
    `kind` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '1 notify feed, 2 ticket, 3 support chat',
    `ref_id` BIGINT NOT NULL DEFAULT 0 COMMENT 'Ticket id / user id / 0 for the feed',
    `thread_id` BIGINT NOT NULL DEFAULT 0 COMMENT 'Forum topic message_thread_id',
    `title` VARCHAR(128) NOT NULL DEFAULT '' COMMENT 'Topic title snapshot',
    `status` TINYINT(1) NOT NULL DEFAULT 1 COMMENT '1 active, 2 closed',
    `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'Create Time',
    `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT 'Update Time',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_telegram_topic_kind_ref` (`chat_id`, `kind`, `ref_id`),
    UNIQUE KEY `uk_telegram_topic_thread` (`chat_id`, `thread_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
