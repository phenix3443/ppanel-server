-- 控制台下发的期望节点版本。节点拉配置时读到它、与自身版本不同就自升级。
-- 空串表示「不管」——不能用它表达「升到最新」，那会让升级时机变得不可预测。
ALTER TABLE "servers"
    ADD COLUMN IF NOT EXISTS "target_version" varchar(64) NOT NULL DEFAULT '';
