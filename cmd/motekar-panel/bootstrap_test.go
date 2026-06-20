package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/motekar/motekar-panel/internal/auth"
)

func TestParseBootstrapAdminOptions(t *testing.T) {
	options, err := parseBootstrapAdminOptions([]string{
		"--email", "owner@example.com",
		"--display-name", "Owner",
		"--password-stdin",
	})
	if err != nil {
		t.Fatalf("parseBootstrapAdminOptions returned error: %v", err)
	}
	if options.email != "owner@example.com" || options.displayName != "Owner" || !options.passwordStdin {
		t.Fatalf("unexpected options: %#v", options)
	}
}

func TestParseBootstrapAdminOptionsRequiresPasswordStdin(t *testing.T) {
	_, err := parseBootstrapAdminOptions([]string{
		"--email", "owner@example.com",
		"--display-name", "Owner",
	})
	if err == nil || !strings.Contains(err.Error(), "--password-stdin") {
		t.Fatalf("expected --password-stdin error, got %v", err)
	}
}

func TestRunBootstrapAdminPassesInputToCreator(t *testing.T) {
	var got auth.BootstrapInput
	var stdout bytes.Buffer

	err := runBootstrapAdmin(
		[]string{"--email", "owner@example.com", "--display-name", "Owner", "--password-stdin"},
		strings.NewReader("correct horse battery staple\n"),
		&stdout,
		func(_ context.Context, input auth.BootstrapInput) (auth.BootstrapAdmin, error) {
			got = input
			return auth.BootstrapAdmin{Email: input.Email}, nil
		},
	)
	if err != nil {
		t.Fatalf("runBootstrapAdmin returned error: %v", err)
	}
	if got.Email != "owner@example.com" || got.DisplayName != "Owner" || got.Password != "correct horse battery staple" {
		t.Fatalf("unexpected bootstrap input: %#v", got)
	}
	if !strings.Contains(stdout.String(), "created first admin: owner@example.com") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRunBootstrapAdminReturnsCreatorError(t *testing.T) {
	want := errors.New("create failed")

	err := runBootstrapAdmin(
		[]string{"--email", "owner@example.com", "--display-name", "Owner", "--password-stdin"},
		strings.NewReader("correct horse battery staple\n"),
		ioDiscard{},
		func(context.Context, auth.BootstrapInput) (auth.BootstrapAdmin, error) {
			return auth.BootstrapAdmin{}, want
		},
	)
	if !errors.Is(err, want) {
		t.Fatalf("expected creator error, got %v", err)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
