package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
)

func runList(ctx context.Context, args []string, mongoURI string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cl, err := connectMongo(ctx, mongoURI)
	if err != nil {
		return err
	}
	defer func() { _ = cl.Close(context.Background()) }()

	repo, err := platform.NewClinicRepository(ctx, cl.Mongo().Database("platform"))
	if err != nil {
		return fmt.Errorf("init clinic repository: %w", err)
	}

	clinics, err := repo.List(ctx)
	if err != nil {
		return fmt.Errorf("list clinics: %w", err)
	}

	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SLUG\tNAME\tTIER\tSTATUS")
	for _, c := range clinics {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", c.Slug, c.Name, c.Tier, c.Status)
	}
	return tw.Flush()
}
