INSERT INTO `system` (`category`, `key`, `value`, `type`, `desc`, `created_at`, `updated_at`)
SELECT 'subscribe', 'ProfileUpdateInterval', '0', 'int64', 'Profile update interval in hours; 0 disables the response header', NOW(), NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM `system` WHERE `category` = 'subscribe' AND `key` = 'ProfileUpdateInterval'
);

INSERT INTO `system` (`category`, `key`, `value`, `type`, `desc`, `created_at`, `updated_at`)
SELECT 'subscribe', 'ProfileWebPageURL', '', 'string', 'Profile web page URL; blank disables the response header', NOW(), NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM `system` WHERE `category` = 'subscribe' AND `key` = 'ProfileWebPageURL'
);
