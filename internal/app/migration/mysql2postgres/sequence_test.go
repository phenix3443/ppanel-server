package mysql2postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"
)

// Copying one table must leave the sequences of every other table alone:
// they belong to data this run did not copy, and in the shared test database
// other packages create and drop tables while it runs.
func TestMigrateResetsOnlyTheCopiedTablesSequences(t *testing.T) {
	mysqlDSN, postgresDSN := os.Getenv("PPANEL_TEST_MYSQL_DSN"), os.Getenv("PPANEL_TEST_POSTGRES_DSN")
	if mysqlDSN == "" || postgresDSN == "" {
		t.Skip("set PPANEL_TEST_MYSQL_DSN and PPANEL_TEST_POSTGRES_DSN to run the sequence scope test")
	}
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	copied, other := fmt.Sprintf("m2p_seq_copied_%d", suffix), fmt.Sprintf("m2p_seq_other_%d", suffix)

	mysqlDB, err := sql.Open("mysql", normalizeMySQLDSN(mysqlDSN, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	defer closeDatabase("mysql", mysqlDB)
	postgresDB, err := sql.Open("pgx", postgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer closeDatabase("postgres", postgresDB)
	exec := func(db *sql.DB, query string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	exec(mysqlDB, "CREATE TABLE "+quoteMySQLIdent(copied)+" (id BIGINT PRIMARY KEY)")
	t.Cleanup(func() { _, _ = mysqlDB.ExecContext(ctx, "DROP TABLE IF EXISTS "+quoteMySQLIdent(copied)) })
	exec(mysqlDB, "INSERT INTO "+quoteMySQLIdent(copied)+" (id) VALUES (7)")
	for _, table := range []string{copied, other} {
		exec(postgresDB, "CREATE TABLE public."+quotePGIdent(table)+" (id bigserial PRIMARY KEY)")
		name := table
		t.Cleanup(func() { _, _ = postgresDB.ExecContext(ctx, "DROP TABLE IF EXISTS public."+quotePGIdent(name)) })
	}
	exec(postgresDB, "SELECT setval(pg_get_serial_sequence('public."+other+"', 'id'), 100)")

	if err := Migrate(ctx, Config{MySQLDSN: mysqlDSN, PostgresDSN: postgresDSN, Schema: "public", Tables: copied, BatchSize: 10, Location: "UTC"}); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	next := func(table string) int64 {
		t.Helper()
		var v int64
		if err := postgresDB.QueryRowContext(ctx, "SELECT nextval(pg_get_serial_sequence('public."+table+"', 'id'))").Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if got := next(copied); got != 8 {
		t.Fatalf("the copied table's sequence continues at %d, want 8 (past the copied id 7)", got)
	}
	if got := next(other); got != 101 {
		t.Fatalf("an uncopied table's sequence was reset: next is %d, want 101", got)
	}
}
