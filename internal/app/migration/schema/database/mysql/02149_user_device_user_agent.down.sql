UPDATE `user_device` SET `user_agent` = LEFT(`user_agent`, 64) WHERE CHAR_LENGTH(`user_agent`) > 64;
ALTER TABLE `user_device`
    MODIFY COLUMN `user_agent` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL COMMENT 'Device User Agent.';
