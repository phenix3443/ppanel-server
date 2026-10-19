package schema

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/perfect-panel/server/pkg/logger/logtest"
	"github.com/perfect-panel/server/pkg/orm"
	"gorm.io/gorm"
)

type fakeVersion struct {
	version uint
	dirty   bool
	err     error
}

func (f *fakeVersion) Version() (uint, bool, error) { return f.version, f.dirty, f.err }

type fakeRepair struct {
	present  bool
	probeErr error
	applied  int
}

func (f *fakeRepair) HasEntitlements() (bool, error) { return f.present, f.probeErr }
func (f *fakeRepair) ApplyEntitlements() error {
	f.applied++
	return nil
}

func TestRepairLegacyForkVersionOnlyAppliesToTheForkState(t *testing.T) {
	logtest.Discard(t)
	cases := []struct {
		name        string
		client      *fakeVersion
		entitlement bool
		wantApply   int
	}{
		{"stopped at the fork's 03000", &fakeVersion{version: 3000}, false, 1},
		{"migrated past it without upstream's 03000", &fakeVersion{version: 3024}, false, 1},
		{"upstream 03000 applied", &fakeVersion{version: 3000}, true, 0},
		{"newer database", &fakeVersion{version: 3024}, true, 0},
		{"dirty is left to an operator", &fakeVersion{version: 3000, dirty: true}, false, 0},
		{"older database", &fakeVersion{version: 2178}, false, 0},
		{"fresh database", &fakeVersion{err: migrate.ErrNilVersion}, false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repair := &fakeRepair{present: tc.entitlement}
			if err := repairLegacyForkVersion(tc.client, repair); err != nil {
				t.Fatal(err)
			}
			if repair.applied != tc.wantApply {
				t.Fatalf("applied %d times, want %d", repair.applied, tc.wantApply)
			}
		})
	}
}

func TestRepairLegacyForkVersionReportsAFailedProbe(t *testing.T) {
	repair := &fakeRepair{probeErr: errors.New("down")}
	if err := repairLegacyForkVersion(&fakeVersion{version: 3000}, repair); err == nil || repair.applied != 0 {
		t.Fatalf("a failed probe must stop the migration without applying: err=%v applied=%d", err, repair.applied)
	}
}

func TestSQLStatementsSplitsTheEntitlementScripts(t *testing.T) {
	for _, dir := range []string{"database/mysql", "database/postgres"} {
		script, err := sqlFiles.ReadFile(dir + "/" + entitlementScript)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(sqlStatements(string(script))); got != 7 {
			t.Fatalf("%s: %d statements, want 7", dir, got)
		}
	}
}

// legacyForkBaseline is the last migration before the colliding 03000.
const legacyForkBaseline = 2178

// A database the fork's old binary migrated (version 3000, target_version
// column, no entitlement tables) must end up with the full schema — both
// when it is still at 3000 and when a binary without the repair already
// migrated it past 3000.
func TestUpRepairsAForkLegacyDatabase(t *testing.T) {
	for driver, env := range map[string]string{orm.DriverMySQL: "PPANEL_TEST_MYSQL_DSN", orm.DriverPostgres: "PPANEL_TEST_POSTGRES_DSN"} {
		for _, pastFork := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/past=%v", driver, pastFork), func(t *testing.T) {
				shared := os.Getenv(env)
				if shared == "" {
					t.Skipf("set %s to run", env)
				}
				// 其他包的连库测试同时在用 shared 那个库，这里只能在自己建的库里折腾
				dsn := scratchDatabase(t, driver, shared)
				db := connectForTest(t, driver, dsn)
				reproduceForkLegacy(t, driver, dsn, db, pastFork)

				if err := Up(driver, dsn); err != nil && !errors.Is(err, NoChange) {
					t.Fatalf("Up on a fork-legacy database: %v", err)
				}
				if !db.Migrator().HasTable(entitlementTable) || !db.Migrator().HasColumn("user_subscribe", "entitlement_source") {
					t.Fatal("upstream's 03000 did not run")
				}
				if !db.Migrator().HasColumn("servers", "target_version") {
					t.Fatal("the target_version column is gone")
				}
				// The repaired database is an ordinary one now.
				if err := Up(driver, dsn); !errors.Is(err, NoChange) {
					t.Fatalf("second Up: %v, want no change", err)
				}
			})
		}
	}
}

// reproduceForkLegacy migrates the database the way the old fork binary did:
// every migration up to 2178, then its own 03000 under that number. With
// pastFork, a binary without the repair then migrates it further, skipping
// upstream's 03000.
func reproduceForkLegacy(t *testing.T, driver, dsn string, db *gorm.DB, pastFork bool) {
	t.Helper()
	client, err := Migrate(driver, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = client.Close() }()
	if err := client.Migrate(legacyForkBaseline); err != nil {
		t.Fatalf("migrate to %d: %v", legacyForkBaseline, err)
	}
	alter := `ALTER TABLE servers ADD COLUMN target_version varchar(64) NOT NULL DEFAULT ''`
	if err := db.Exec(alter).Error; err != nil {
		t.Fatalf("apply the fork's 03000: %v", err)
	}
	if err := client.Force(legacyForkVersion); err != nil {
		t.Fatal(err)
	}
	if !pastFork {
		return
	}
	if err := client.Up(); err != nil {
		t.Fatalf("plain Up past the fork's 03000: %v", err)
	}
	if db.Migrator().HasTable(entitlementTable) {
		t.Fatal("the reproduction is wrong: upstream's 03000 ran")
	}
}

func connectForTest(t *testing.T, driver, dsn string) *gorm.DB {
	t.Helper()
	cfg := orm.ParseDSN(dsn)
	if cfg == nil {
		t.Fatalf("%s dsn parse failed", driver)
	}
	cfg.Driver = orm.NormalizeDriver(driver)
	db, err := orm.ConnectDatabase(orm.Mysql{Config: *cfg})
	if err != nil {
		t.Fatalf("%s connect failed: %v", driver, err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	return db
}

// scratchDatabase creates an empty database next to the one shared names and
// returns its DSN; the database is dropped when the test ends.
func scratchDatabase(t *testing.T, driver, shared string) string {
	t.Helper()
	admin := connectForTest(t, driver, shared)
	name := fmt.Sprintf("ppanel_legacy_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatalf("create scratch database: %v", err)
	}
	t.Cleanup(func() {
		drop := "DROP DATABASE " + name
		if orm.NormalizeDriver(driver) == orm.DriverPostgres {
			drop += " WITH (FORCE)"
		}
		if err := admin.Exec(drop).Error; err != nil {
			t.Errorf("drop scratch database: %v", err)
		}
	})
	cfg := orm.ParseDSN(shared)
	if cfg == nil {
		t.Fatalf("%s dsn parse failed", driver)
	}
	cfg.Driver = orm.NormalizeDriver(driver)
	cfg.Dbname = name
	return orm.Mysql{Config: *cfg}.MigrationDsn()
}
