package schema

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationLineGuardAcrossDirectoryMove(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for migration-line fixtures")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is required for the migration-line guard")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "script", "check-migration-lines.sh"))
	if err != nil {
		t.Fatal(err)
	}
	const oldPath = "initialize/migrate/database"
	const newPath = "internal/app/migration/schema/database"
	for _, tc := range []struct {
		name, baseline, change, wantError string
	}{
		{"legacy reference", oldPath, "", ""},
		{"relocated reference", newPath, "", ""},
		{"missing LTS migration", oldPath, "missing", "exist on HEAD but not here"},
		{"missing dialect", oldPath, "asymmetric", "missing one of the two dialects"},
		{"low numbered feature", oldPath, "02002", "exists only on this line but is numbered below"},
		{"feature band", oldPath, "03000", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			git := func(args ...string) {
				t.Helper()
				options := []string{"-c", "user.name=Migration Test", "-c", "user.email=migration@example.test", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=" + filepath.Join(repo, "disabled-hooks")}
				cmd := exec.Command("git", append(options, args...)...)
				cmd.Dir = repo
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v\n%s", args, err, output)
				}
			}
			writeMigration := func(base, dialect, version string) {
				t.Helper()
				path := filepath.Join(repo, base, dialect, version+"_fixture.up.sql")
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("SELECT 1;\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			git("init", "--quiet")
			for _, dialect := range []string{"mysql", "postgres"} {
				for _, version := range []string{"02000", "02001"} {
					writeMigration(tc.baseline, dialect, version)
				}
			}
			git("add", ".")
			git("commit", "--quiet", "-m", "migration baseline")
			if tc.baseline != newPath {
				if err := os.MkdirAll(filepath.Join(repo, filepath.Dir(newPath)), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(filepath.Join(repo, tc.baseline), filepath.Join(repo, newPath)); err != nil {
					t.Fatal(err)
				}
			}
			switch tc.change {
			case "missing", "asymmetric":
				dialects := []string{"postgres"}
				if tc.change == "missing" {
					dialects = append(dialects, "mysql")
				}
				for _, dialect := range dialects {
					if err := os.Remove(filepath.Join(repo, newPath, dialect, "02001_fixture.up.sql")); err != nil {
						t.Fatal(err)
					}
				}
			case "02002", "03000":
				for _, dialect := range []string{"mysql", "postgres"} {
					writeMigration(newPath, dialect, tc.change)
				}
			}
			cmd := exec.Command("bash", script)
			cmd.Dir = repo
			cmd.Env = append(os.Environ(), "LTS_REF=HEAD")
			output, err := cmd.CombinedOutput()
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("compatible migration lines rejected: %v\n%s", err, output)
				}
			} else if err == nil || !strings.Contains(string(output), tc.wantError) {
				t.Fatalf("wanted rejection %q, got err=%v\n%s", tc.wantError, err, output)
			}
		})
	}
}
