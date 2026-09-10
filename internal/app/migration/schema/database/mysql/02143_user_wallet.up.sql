-- Billing-owned wallet table (ADR-001 step 5): the money columns move off
-- the identity-owned user row.  During the transition every wallet movement
-- dual-writes both places and readers keep using the user row; once they
-- migrate to the wallet view the user columns are dropped.
CREATE TABLE `user_wallet` (
    `user_id` BIGINT NOT NULL COMMENT 'User Id (identity reference, no FK across domains)',
    `balance` BIGINT NOT NULL DEFAULT 0 COMMENT 'User Balance Amount',
    `gift_amount` BIGINT NOT NULL DEFAULT 0 COMMENT 'User Gift Amount',
    `commission` BIGINT NOT NULL DEFAULT 0 COMMENT 'Commission Amount',
    `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

INSERT INTO `user_wallet` (`user_id`, `balance`, `gift_amount`, `commission`)
SELECT `id`, `balance`, `gift_amount`, `commission` FROM `user`;
