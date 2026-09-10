ALTER TABLE `task`
    ADD COLUMN `daily_date` varchar(10) NOT NULL DEFAULT '' AFTER `current`,
    ADD COLUMN `daily_sent` bigint unsigned NOT NULL DEFAULT 0 AFTER `daily_date`;
