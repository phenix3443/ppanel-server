-- Billing-owned wallet table (ADR-001 step 5): the money columns move off
-- the identity-owned user row.  During the transition every wallet movement
-- dual-writes both places and readers keep using the user row; once they
-- migrate to the wallet view the user columns are dropped.
CREATE TABLE user_wallet (
    user_id BIGINT NOT NULL,
    balance BIGINT NOT NULL DEFAULT 0,
    gift_amount BIGINT NOT NULL DEFAULT 0,
    commission BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id)
);

COMMENT ON COLUMN user_wallet.user_id IS 'User Id (identity reference, no FK across domains)';
COMMENT ON COLUMN user_wallet.balance IS 'User Balance Amount';
COMMENT ON COLUMN user_wallet.gift_amount IS 'User Gift Amount';
COMMENT ON COLUMN user_wallet.commission IS 'Commission Amount';

INSERT INTO user_wallet (user_id, balance, gift_amount, commission)
SELECT id, balance, gift_amount, commission FROM "user";
