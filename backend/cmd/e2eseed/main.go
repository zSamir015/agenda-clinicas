// Command e2eseed provisions a synthetic clinic, template roles, and a one-time
// owner invitation in MongoDB for Playwright end-to-end tests.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
	"github.com/zSamir015/agenda-clinicas/backend/internal/rbac"
	"github.com/zSamir015/agenda-clinicas/backend/internal/users"
)

type seedOutput struct {
	ClinicSlug  string `json:"clinicSlug"`
	ClinicName  string `json:"clinicName"`
	Email       string `json:"email"`
	InviteToken string `json:"inviteToken"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017/?replicaSet=rs0"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cl, err := mongodb.Connect(ctx, mongoURI)
	if err != nil {
		return err
	}
	defer func() { _ = cl.Close(context.Background()) }()

	platformDB := cl.Mongo().Database("platform")
	clinics, err := platform.NewClinicRepository(ctx, platformDB)
	if err != nil {
		return fmt.Errorf("init clinic repo: %w", err)
	}
	roles, err := rbac.NewRoleRepository(ctx, platformDB)
	if err != nil {
		return fmt.Errorf("init role repo: %w", err)
	}
	userRepo, err := users.NewRepository(ctx, platformDB)
	if err != nil {
		return fmt.Errorf("init users repo: %w", err)
	}

	suffix := time.Now().UTC().Format("20060102150405.000000")
	slug := fmt.Sprintf("clinica-e2e-%d", time.Now().UnixNano())
	name := "Clínica E2E " + suffix
	email := fmt.Sprintf("owner-%d@demo.example", time.Now().UnixNano())

	clinicInput, err := platform.NewClinic(slug, name, platform.TierStandard, "pa")
	if err != nil {
		return fmt.Errorf("build clinic: %w", err)
	}
	clinic, err := clinics.Create(ctx, clinicInput)
	if err != nil {
		return fmt.Errorf("create clinic: %w", err)
	}
	if err := roles.EnsureTemplates(ctx, clinic.ID); err != nil {
		return fmt.Errorf("ensure templates: %w", err)
	}
	ownerRole, err := roles.ByName(ctx, clinic.ID, rbac.RoleOwner)
	if err != nil {
		return fmt.Errorf("lookup owner role: %w", err)
	}

	_, rawToken, err := userRepo.CreateInvitation(
		ctx,
		clinic.ID,
		email,
		"Dra. Sofía E2E",
		ownerRole.ID,
		bson.NewObjectID(),
		time.Hour,
	)
	if err != nil {
		return fmt.Errorf("create invitation: %w", err)
	}

	return json.NewEncoder(os.Stdout).Encode(seedOutput{
		ClinicSlug:  clinic.Slug,
		ClinicName:  clinic.Name,
		Email:       email,
		InviteToken: rawToken,
	})
}
