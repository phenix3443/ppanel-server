package schema

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/orm"
	"gorm.io/gorm"
)

// 【本 fork 独有，别删】fork 曾把 server_target_version 编成 03000，上游后来也用
// 03000 发了 subscription_entitlements；同步上游时 fork 那份改号成了 03024。
// 跑过旧版的库记着版本 3000，golang-migrate 会把上游的 03000 当成已执行而跳过，
// 接着把 3001 往后的照常跑完——于是库可能停在 3000，也可能已经到了 3024，
// 但 user_subscribe.entitlement_source 和三张权益表都没有，订阅查询全部报错。
//
// 判据：版本 ≥ 3000、不 dirty、且没有 subscription_entitlement 表（上游正常跑过
// 03000 的库一定有它）。命中就直接执行上游 03000 的 up 脚本，版本号不动：
// 3001 往后的迁移都不碰这几张表，补在它们之后执行没有区别；库若停在 3000，
// 随后的 Up 照常跑到最新，03024 对已有的 target_version 列是空操作。
const (
	legacyForkVersion = 3000
	entitlementTable  = "subscription_entitlement"
	entitlementScript = "03000_subscription_entitlements.up.sql"
)

// legacyVersionReader is the part of a migration client the repair reads.
type legacyVersionReader interface {
	Version() (uint, bool, error)
}

// legacyRepair is the database side of the repair: whether upstream's 03000
// ran, and running it.
type legacyRepair interface {
	HasEntitlements() (bool, error)
	ApplyEntitlements() error
}

// repairLegacyForkVersion applies upstream's 03000 to a database the fork's
// old 03000 made golang-migrate skip it on.
func repairLegacyForkVersion(client legacyVersionReader, repair legacyRepair) error {
	version, dirty, err := client.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return nil // a fresh database: nothing was applied under the old numbering.
	}
	if err != nil {
		return fmt.Errorf("read migration version: %w", err)
	}
	if version < legacyForkVersion || dirty {
		return nil
	}
	present, err := repair.HasEntitlements()
	if err != nil {
		return fmt.Errorf("probe %s: %w", entitlementTable, err)
	}
	if present {
		return nil // upstream's 03000 did run.
	}
	logger.Infow("[Migrate] upstream's 03000 was skipped behind the fork's old 03000; applying it",
		logger.Field("version", version))
	if err := repair.ApplyEntitlements(); err != nil {
		return fmt.Errorf("apply %s: %w", entitlementScript, err)
	}
	return nil
}

// dbLegacyRepair runs the repair against the database the DSN names.
type dbLegacyRepair struct {
	driver, dsn string
}

func (r dbLegacyRepair) HasEntitlements() (present bool, err error) {
	err = r.withDB(func(db *gorm.DB) error {
		present = db.Migrator().HasTable(entitlementTable)
		return nil
	})
	return present, err
}

// ApplyEntitlements runs the embedded script one statement at a time (the
// MySQL connection does not enable multi-statements) inside a transaction,
// which PostgreSQL honours for DDL; MySQL commits each DDL statement itself.
func (r dbLegacyRepair) ApplyEntitlements() error {
	script, err := sqlFiles.ReadFile(r.sourcePath() + "/" + entitlementScript)
	if err != nil {
		return err
	}
	return r.withDB(func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			for _, stmt := range sqlStatements(string(script)) {
				if err := tx.Exec(stmt).Error; err != nil {
					return err
				}
			}
			return nil
		})
	})
}

func (r dbLegacyRepair) sourcePath() string {
	if orm.NormalizeDriver(r.driver) == orm.DriverPostgres {
		return "database/postgres"
	}
	return "database/mysql"
}

func (r dbLegacyRepair) withDB(fn func(*gorm.DB) error) error {
	cfg := orm.ParseDSN(r.dsn)
	if cfg == nil {
		return errors.New("parse database dsn")
	}
	cfg.Driver = orm.NormalizeDriver(r.driver)
	db, err := orm.ConnectDatabase(orm.Mysql{Config: *cfg})
	if err != nil {
		return err
	}
	if sqlDB, err := db.DB(); err == nil {
		defer func() { _ = sqlDB.Close() }()
	}
	return fn(db)
}

// sqlStatements splits a migration script on statement-ending semicolons,
// dropping comment lines. The 03000 scripts hold no semicolon in a literal.
func sqlStatements(script string) []string {
	var lines []string
	for _, line := range strings.Split(script, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			lines = append(lines, line)
		}
	}
	var stmts []string
	for _, stmt := range strings.Split(strings.Join(lines, "\n"), ";") {
		if stmt = strings.TrimSpace(stmt); stmt != "" {
			stmts = append(stmts, stmt)
		}
	}
	return stmts
}
