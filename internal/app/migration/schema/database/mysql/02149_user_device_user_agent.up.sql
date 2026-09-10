-- Modern browser User-Agent strings routinely exceed the original
-- varchar(64), which made every device bind fail with "Data too long".
ALTER TABLE `user_device`
    MODIFY COLUMN `user_agent` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL COMMENT 'Device User Agent.';
