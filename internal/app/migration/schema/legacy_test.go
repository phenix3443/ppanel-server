package schema

import (
	"errors"
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/perfect-panel/server/pkg/logger/logtest"
	"github.com/perfect-panel/server/pkg/orm"
	"gorm.io/gorm"
)

type fakeVersion struct {
	version uint
	dirty   bool
	err     error
	forced  []int
}

func (f *fakeVersion) Version() (uint, bool, error) { return f.version, f.dirty, f.err }
func (f *fakeVersion) Force(v int) error {
	f.forced = append(f.forced, v)
	return nil
}

func TestRepairLegacyForkVersionOnlyRewindsTheForkState(t *testing.T) {
	logtest.Discard(t)
	tables := func(present bool) func() (bool, error) {
		return func() (bool, error) { return present, nil }
	}
	cases := []struct {
		name        string
		client      *fakeVersion
		entitlement bool
		wantForce   []int
	}{
		{"fork 03000 applied", &fakeVersion{version: 3000}, false, []int{2178}},
		{"upstream 03000 applied", &fakeVersion{version: 3000}, true, nil},
		{"dirty at 3000 is left to an operator", &fakeVersion{version: 3000, dirty: true}, false, nil},
		{"older database", &fakeVersion{version: 2178}, false, nil},
		{"newer database", &fakeVersion{version: 3024}, true, nil},
		{"fresh database", &fakeVersion{err: migrate.ErrNilVersion}, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := repairLegacyForkVersion(tc.client, tables(tc.entitlement)); err != nil {
				t.Fatal(err)
			}
			if len(tc.client.forced) != len(tc.wantForce) || (len(tc.wantForce) == 1 && tc.client.forced[0] != tc.wantForce[0]) {
				t.Fatalf("forced %v, want %v", tc.client.forced, tc.wantForce)
			}
		})
	}
}

func TestRepairLegacyForkVersionReportsAFailedProbe(t *testing.T) {
	probe := func() (bool, error) { return false, errors.New("down") }
	client := &fakeVersion{version: 3000}
	if err := repairLegacyForkVersion(client, probe); err == nil || len(client.forced) != 0 {
		t.Fatalf("a failed probe must stop the migration without rewinding: err=%v forced=%v", err, client.forced)
	}
}

// A database the fork's old binary migrated (version 3000, target_version
// column, no entitlement tables) must end up with the full schema.
func TestUpRepairsAForkLegacyDatabase(t *testing.T) {
	for driver, env := range map[string]string{orm.DriverMySQL: "PPANEL_TEST_MYSQL_DSN", orm.DriverPostgres: "PPANEL_TEST_POSTGRES_DSN"} {
		t.Run(driver, func(t *testing.T) {
			dsn := os.Getenv(env)
			if dsn == "" {
				t.Skipf("set %s to run", env)
			}
			client, err := Migrate(driver, dsn)
			if err != nil {
				t.Fatal(err)
			}
			if err := client.Drop(); err != nil {
				t.Fatalf("drop: %v", err)
			}
			_, _ = client.Close()
			// Reproduce the old fork binary: every migration up to 2178, then
			// its own 03000 under that number.
			client, err = Migrate(driver, dsn)
			if err != nil {
				t.Fatal(err)
			}
			if err := client.Migrate(legacyForkBaseline); err != nil {
				t.Fatalf("migrate to %d: %v", legacyForkBaseline, err)
			}
			_, _ = client.Close()
			db := connectForTest(t, driver, dsn)
			alter := `ALTER TABLE servers ADD COLUMN target_version varchar(64) NOT NULL DEFAULT ''`
			if err := db.Exec(alter).Error; err != nil {
				t.Fatalf("apply the fork's 03000: %v", err)
			}
			client, err = Migrate(driver, dsn)
			if err != nil {
				t.Fatal(err)
			}
			if err := client.Force(legacyForkVersion); err != nil {
				t.Fatal(err)
			}
			_, _ = client.Close()

			if err := Up(driver, dsn); err != nil && !errors.Is(err, NoChange) {
				t.Fatalf("Up on a fork-legacy database: %v", err)
			}
			if !db.Migrator().HasTable(entitlementTable) || !db.Migrator().HasColumn("user_subscribe", "entitlement_source") {
				t.Fatal("upstream's 03000 did not run")
			}
			if !db.Migrator().HasColumn("servers", "target_version") {
				t.Fatal("the target_version column is gone")
			}
		})
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
