-- Forum-topic mapping for the Telegram administrator group; see the MySQL
-- variant for the design notes.
CREATE TABLE telegram_topic (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL DEFAULT 0,
    kind SMALLINT NOT NULL DEFAULT 0,
    ref_id BIGINT NOT NULL DEFAULT 0,
    thread_id BIGINT NOT NULL DEFAULT 0,
    title VARCHAR(128) NOT NULL DEFAULT '',
    status SMALLINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uk_telegram_topic_kind_ref ON telegram_topic (chat_id, kind, ref_id);
CREATE UNIQUE INDEX uk_telegram_topic_thread ON telegram_topic (chat_id, thread_id);
