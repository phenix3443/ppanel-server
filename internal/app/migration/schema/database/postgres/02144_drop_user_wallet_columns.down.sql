ALTER TABLE "user"
    ADD COLUMN balance BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN gift_amount BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN commission BIGINT NOT NULL DEFAULT 0;

UPDATE "user" u
SET balance = w.balance,
    gift_amount = w.gift_amount,
    commission = w.commission
FROM user_wallet w
WHERE w.user_id = u.id;
