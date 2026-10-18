package schema

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/orm"
)

// 【本 fork 独有，别删】fork 曾把 server_target_version 编成 03000，上游后来也用
// 03000 发了 subscription_entitlements；同步上游时 fork 那份改号成了 03024。
// 跑过旧版的库停在版本 3000，golang-migrate 会把上游的 03000 当成已执行而跳过，
// 于是 user_subscribe.entitlement_source 那几张表永远建不出来，订阅查询全部报错。
//
// 判据只认这一种状态：版本 3000、不 dirty、且没有 subscription_entitlement 表
// （上游正常跑到 3000 的库一定有它）。命中就把版本退回 03000 之前的 2178 再往上迁：
// 03000 补建权益表，03024 的 ADD COLUMN IF NOT EXISTS 对已有的列是空操作。
const (
	legacyForkVersion  = 3000
	legacyForkBaseline = 2178
	entitlementTable   = "subscription_entitlement"
)

// legacyVersionReader is the part of a migration client the repair reads and
// rewrites.
type legacyVersionReader interface {
	Version() (uint, bool, error)
	Force(version int) error
}

// repairLegacyForkVersion rewinds a database the fork's old 03000 migration
// left at version 3000, so the upstream migration of that number runs.
// hasEntitlements reports whether the entitlement table exists.
func repairLegacyForkVersion(client legacyVersionReader, hasEntitlements func() (bool, error)) error {
	version, dirty, err := client.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return nil // a fresh database: nothing was applied under the old numbering.
	}
	if err != nil {
		return fmt.Errorf("read migration version: %w", err)
	}
	if version != legacyForkVersion || dirty {
		return nil
	}
	present, err := hasEntitlements()
	if err != nil {
		return fmt.Errorf("probe %s: %w", entitlementTable, err)
	}
	if present {
		return nil // upstream's 03000 did run.
	}
	logger.Infow("[Migrate] database is at the fork's old migration 03000; rewinding so upstream's 03000 runs",
		logger.Field("from", legacyForkVersion), logger.Field("to", legacyForkBaseline))
	if err := client.Force(legacyForkBaseline); err != nil {
		return fmt.Errorf("rewind legacy migration version: %w", err)
	}
	return nil
}

// entitlementProbe reports whether the entitlement table exists in the
// database the DSN names.
func entitlementProbe(driver, dsn string) func() (bool, error) {
	return func() (bool, error) {
		cfg := orm.ParseDSN(dsn)
		if cfg == nil {
			return false, errors.New("parse database dsn")
		}
		cfg.Driver = orm.NormalizeDriver(driver)
		db, err := orm.ConnectDatabase(orm.Mysql{Config: *cfg})
		if err != nil {
			return false, err
		}
		if sqlDB, err := db.DB(); err == nil {
			defer func() { _ = sqlDB.Close() }()
		}
		return db.Migrator().HasTable(entitlementTable), nil
	}
}
