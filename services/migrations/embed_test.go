package migrations

import (
	"io/fs"
	"testing"
)

func TestEmbeddedMigrationsContainSQL(t *testing.T) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		t.Fatalf("read embedded FS: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("embedded migrations FS is empty")
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("embedded migration %q must be a file", entry.Name())
		}
		content, err := fs.ReadFile(FS, entry.Name())
		if err != nil {
			t.Fatalf("read embedded migration %q: %v", entry.Name(), err)
		}
		if len(content) == 0 {
			t.Fatalf("embedded migration %q is empty", entry.Name())
		}
	}
}
