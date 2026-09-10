ALTER TABLE "order"
    ADD COLUMN guest_auth_type VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN guest_identifier VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN guest_password_hash VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN guest_invite_code VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN guest_checkout_token_hash CHAR(64) NOT NULL DEFAULT '';
