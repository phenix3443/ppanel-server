-- ADR-001 step 5 endgame: the money columns leave the identity-owned user
-- row.  Every reader and writer now uses the billing-owned user_wallet
-- table (backfilled by 02143), so the legacy columns drop.
ALTER TABLE `user`
    DROP COLUMN `balance`,
    DROP COLUMN `gift_amount`,
    DROP COLUMN `commission`;
