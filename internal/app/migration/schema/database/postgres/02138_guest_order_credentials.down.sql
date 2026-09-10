ALTER TABLE "order"
    DROP COLUMN guest_checkout_token_hash,
    DROP COLUMN guest_invite_code,
    DROP COLUMN guest_password_hash,
    DROP COLUMN guest_identifier,
    DROP COLUMN guest_auth_type;
