// Package main implements the tenantctl CLI for creating and listing clinics.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
)

const defaultMongoURI = "mongodb://localhost:27017/?replicaSet=rs0"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("subcommand required (create, list)")
	}

	mongoURI := getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = defaultMongoURI
	}

	switch args[0] {
	case "create":
		return runCreate(ctx, args[1:], mongoURI, getenv, stdout, stderr)
	case "list":
		return runList(ctx, args[1:], mongoURI, stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: tenantctl <create|list> [flags]")
	_, _ = fmt.Fprintln(w, "  create  Create a clinic, ensure its data key, and run migrations")
	_, _ = fmt.Fprintln(w, "  list    List registered clinics")
}

func connectMongo(ctx context.Context, uri string) (*mongodb.Client, error) {
	cl, err := mongodb.Connect(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("connect mongodb: %w", err)
	}
	return cl, nil
}
