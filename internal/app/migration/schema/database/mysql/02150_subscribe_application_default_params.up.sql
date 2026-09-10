-- Template params were readable only from the subscription URL's query string,
-- so a client whose template expects something like `mode=rule` had no way to
-- receive it unless every user appended it by hand. Each application now carries
-- its own defaults, which a request's query string still overrides.
ALTER TABLE `subscribe_application`
    ADD COLUMN `default_params` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Default Template Params';
