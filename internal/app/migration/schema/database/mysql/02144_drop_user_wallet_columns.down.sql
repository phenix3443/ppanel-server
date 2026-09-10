ALTER TABLE `user`
    ADD COLUMN `balance` BIGINT NOT NULL DEFAULT 0 COMMENT 'User Balance',
    ADD COLUMN `gift_amount` BIGINT NOT NULL DEFAULT 0 COMMENT 'User Gift Amount',
    ADD COLUMN `commission` BIGINT NOT NULL DEFAULT 0 COMMENT 'Commission';

UPDATE `user` u
JOIN `user_wallet` w ON w.`user_id` = u.`id`
SET u.`balance` = w.`balance`,
    u.`gift_amount` = w.`gift_amount`,
    u.`commission` = w.`commission`;
