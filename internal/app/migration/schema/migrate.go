package schema

import (
	"embed"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/orm"
)

//go:embed database/mysql/*.sql database/postgres/*.sql
var sqlFiles embed.FS
var NoChange = migrate.ErrNoChange

// Up applies every pending migration and always releases the source and
// database drivers opened by golang-migrate. Callers previously invoked
// Migrate(...).Up() directly and leaked the migration driver's independent
// database connection after startup or installation.
func Up(driver, dsn string) error {
	return upAndClose(Migrate(driver, dsn))
}

type migrationRunner interface {
	Up() error
	Close() (sourceErr, databaseErr error)
}

func upAndClose(client migrationRunner) error {
	migrateErr := client.Up()
	sourceErr, databaseErr := client.Close()
	closeErr := errors.Join(sourceErr, databaseErr)
	if closeErr == nil {
		return migrateErr
	}

	closeErr = fmt.Errorf("close migration drivers: %w", closeErr)
	if migrateErr == nil || errors.Is(migrateErr, migrate.ErrNoChange) {
		return closeErr
	}
	return errors.Join(migrateErr, closeErr)
}

func Migrate(driver, dsn string) *migrate.Migrate {
	driver = orm.NormalizeDriver(driver)
	sourcePath := "database/mysql"
	databaseURL := dsn
	switch driver {
	case orm.DriverMySQL:
		databaseURL = ensureScheme(orm.DriverMySQL, dsn)
	case orm.DriverPostgres:
		sourcePath = "database/postgres"
		databaseURL = ensureScheme(orm.DriverPostgres, dsn)
	default:
		logger.Errorf("[Migrate] unsupported database driver: %s", driver)
		panic(fmt.Errorf("unsupported database driver: %s", driver))
	}
	d, err := iofs.New(sqlFiles, sourcePath)
	if err != nil {
		logger.Errorf("[Migrate] iofs.New error: %v", err.Error())
		panic(err)
	}
	client, err := migrate.NewWithSourceInstance("iofs", d, databaseURL)
	if err != nil {
		logger.Errorf("[Migrate] NewWithSourceInstance error: %v", err.Error())
		panic(err)
	}
	return client
}

func ensureScheme(driver, dsn string) string {
	if strings.Contains(dsn, "://") {
		return dsn
	}
	return fmt.Sprintf("%s://%s", driver, dsn)
}
