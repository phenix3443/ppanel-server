-- 控制台下发的期望节点版本。节点拉配置时读到它、与自身版本不同就自升级。
-- 空串表示「不管」——不能用它表达「升到最新」，那会让升级时机变得不可预测。
--
-- 【必须幂等】这一列在 fork 旧版里是以 03000 建的，从旧版升级上来的库已经有它
-- （见 schema/legacy.go）。MySQL 8 的 ADD COLUMN 没有 IF NOT EXISTS，只能先查再加。
SET @target_version_exists := (
    SELECT COUNT(*) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'servers' AND column_name = 'target_version'
);
SET @target_version_ddl := IF(@target_version_exists = 0,
    'ALTER TABLE `servers` ADD COLUMN `target_version` varchar(64) NOT NULL DEFAULT ''''',
    'DO 0');
PREPARE target_version_stmt FROM @target_version_ddl;
EXECUTE target_version_stmt;
DEALLOCATE PREPARE target_version_stmt;
