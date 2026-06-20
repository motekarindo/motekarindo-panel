package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/motekar/motekar-panel/internal/auth"
	"github.com/motekar/motekar-panel/internal/config"
	"github.com/motekar/motekar-panel/internal/database"
)

type bootstrapAdminOptions struct {
	email         string
	displayName   string
	passwordStdin bool
}

type bootstrapAdminCreator func(context.Context, auth.BootstrapInput) (auth.BootstrapAdmin, error)

func bootstrap(args []string) error {
	if len(args) == 0 || args[0] != "admin" {
		return fmt.Errorf("usage: motekar-panel bootstrap admin --email <email> --display-name <name> --password-stdin")
	}

	return runBootstrapAdmin(args[1:], os.Stdin, os.Stdout, createBootstrapAdmin)
}

func createBootstrapAdmin(ctx context.Context, input auth.BootstrapInput) (auth.BootstrapAdmin, error) {
	cfg, err := config.LoadPanel()
	if err != nil {
		return auth.BootstrapAdmin{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	db, err := database.OpenPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		return auth.BootstrapAdmin{}, err
	}
	defer db.Close()

	return auth.NewBootstrapService(auth.NewSQLBootstrapStore(db)).CreateFirstAdmin(ctx, input)
}

func runBootstrapAdmin(args []string, stdin io.Reader, stdout io.Writer, create bootstrapAdminCreator) error {
	options, err := parseBootstrapAdminOptions(args)
	if err != nil {
		return err
	}

	password, err := readBootstrapPassword(stdin, options)
	if err != nil {
		return err
	}

	admin, err := create(context.Background(), auth.BootstrapInput{
		Email:       options.email,
		DisplayName: options.displayName,
		Password:    password,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "created first admin: %s\n", admin.Email)
	return nil
}

func parseBootstrapAdminOptions(args []string) (bootstrapAdminOptions, error) {
	flags := flag.NewFlagSet("bootstrap admin", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	var options bootstrapAdminOptions
	flags.StringVar(&options.email, "email", "", "admin email")
	flags.StringVar(&options.displayName, "display-name", "", "admin display name")
	flags.BoolVar(&options.passwordStdin, "password-stdin", false, "read password from stdin")

	if err := flags.Parse(args); err != nil {
		return bootstrapAdminOptions{}, err
	}
	if flags.NArg() != 0 {
		return bootstrapAdminOptions{}, fmt.Errorf("unexpected bootstrap admin arguments: %s", strings.Join(flags.Args(), " "))
	}
	if options.email == "" {
		return bootstrapAdminOptions{}, fmt.Errorf("bootstrap admin email is required")
	}
	if options.displayName == "" {
		return bootstrapAdminOptions{}, fmt.Errorf("bootstrap admin display name is required")
	}
	if !options.passwordStdin {
		return bootstrapAdminOptions{}, fmt.Errorf("bootstrap admin password must be provided with --password-stdin")
	}

	return options, nil
}

func readBootstrapPassword(stdin io.Reader, options bootstrapAdminOptions) (string, error) {
	if !options.passwordStdin {
		return "", fmt.Errorf("bootstrap admin password must be provided with --password-stdin")
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return "", err
	}
	password := strings.TrimRight(string(raw), "\r\n")
	if password == "" {
		return "", fmt.Errorf("bootstrap admin password cannot be empty")
	}
	return password, nil
}
