package database

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"testing/fstest"
)

func TestLoadMigrationsSortsByVersion(t *testing.T) {
	migrations, err := LoadMigrations(fstest.MapFS{
		"000002_second.sql": {Data: []byte("SELECT 2;")},
		"000001_first.sql":  {Data: []byte("SELECT 1;")},
		"README.md":         {Data: []byte("ignored")},
	})
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}

	got := []int{migrations[0].Version, migrations[1].Version}
	want := []int{1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("versions = %v, want %v", got, want)
	}
}

func TestLoadMigrationsRejectsDuplicateVersion(t *testing.T) {
	_, err := LoadMigrations(fstest.MapFS{
		"000001_first.sql": {Data: []byte("SELECT 1;")},
		"1_second.sql":     {Data: []byte("SELECT 2;")},
	})
	if err == nil {
		t.Fatal("expected duplicate version error")
	}
}

func TestRunnerAppliesOnlyPendingMigrations(t *testing.T) {
	store := &memoryStore{applied: map[int]bool{1: true}}
	runner := NewRunner(store)

	ran, err := runner.Up(context.Background(), []Migration{
		{Version: 1, Name: "first", SQL: "SELECT 1;"},
		{Version: 2, Name: "second", SQL: "SELECT 2;"},
	})
	if err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	if len(ran) != 1 || ran[0].Version != 2 {
		t.Fatalf("ran = %#v, want only version 2", ran)
	}
	if !store.ensureCalled {
		t.Fatal("Ensure was not called")
	}
	if !store.applied[2] {
		t.Fatal("version 2 was not marked applied")
	}
}

func TestRunnerStopsOnApplyError(t *testing.T) {
	store := &memoryStore{
		applied:  make(map[int]bool),
		failNext: errors.New("boom"),
	}
	runner := NewRunner(store)

	ran, err := runner.Up(context.Background(), []Migration{
		{Version: 1, Name: "first", SQL: "SELECT 1;"},
	})
	if err == nil {
		t.Fatal("expected apply error")
	}
	if len(ran) != 0 {
		t.Fatalf("ran = %#v, want none", ran)
	}
}

type memoryStore struct {
	ensureCalled bool
	applied      map[int]bool
	failNext     error
}

func (s *memoryStore) Ensure(context.Context) error {
	s.ensureCalled = true
	return nil
}

func (s *memoryStore) AppliedVersions(context.Context) (map[int]bool, error) {
	return s.applied, nil
}

func (s *memoryStore) Apply(_ context.Context, migration Migration) error {
	if s.failNext != nil {
		return s.failNext
	}
	s.applied[migration.Version] = true
	return nil
}
