-- Compensating migration for the October 2025 renumbering (02115_user_algo ->
-- 02116, 02116_site_custom_data -> 02117): databases whose schema_migrations
-- version already sat at the old 02115/02116 never executed the renumbered
-- 02115_ads, so they lack the ads.description column. Same guarded ALTER as
-- 02115; a no-op everywhere the column already exists.
SET
@col_exists := (
    SELECT COUNT(*)
    FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ads'
      AND COLUMN_NAME = 'description'
);

SET
@query := IF(
    @col_exists = 0,
    'ALTER TABLE `ads` ADD COLUMN `description` VARCHAR(255) DEFAULT '''' COMMENT ''Description'';',
    'SELECT "Column `description` already exists"'
);

PREPARE stmt FROM @query;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
