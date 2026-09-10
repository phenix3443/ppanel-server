ALTER TABLE `order`
    ADD COLUMN `guest_auth_type` VARCHAR(255) NOT NULL DEFAULT '' AFTER `subscribe_token`,
    ADD COLUMN `guest_identifier` VARCHAR(255) NOT NULL DEFAULT '' AFTER `guest_auth_type`,
    ADD COLUMN `guest_password_hash` VARCHAR(255) NOT NULL DEFAULT '' AFTER `guest_identifier`,
    ADD COLUMN `guest_invite_code` VARCHAR(255) NOT NULL DEFAULT '' AFTER `guest_password_hash`,
    ADD COLUMN `guest_checkout_token_hash` CHAR(64) NOT NULL DEFAULT '' AFTER `guest_invite_code`;
