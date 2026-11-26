package tenantdb_test

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	_ "github.com/zSamir015/agenda-clinicas/backend/internal/appointments"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb/mongotest"
	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
	_ "github.com/zSamir015/agenda-clinicas/backend/internal/schedule"
	"github.com/zSamir015/agenda-clinicas/backend/internal/tenantdb"
)

type sampleDoc struct {
	ID       bson.ObjectID `bson:"_id,omitempty"`
	ClinicID bson.ObjectID `bson:"clinicId,omitempty"`
	Marker   string        `bson:"marker"`
}

type sampleProbe struct {
	repo *tenantdb.Repo[sampleDoc]
}

func (p *sampleProbe) InsertSample(ctx context.Context, marker string) error {
	return p.repo.Insert(ctx, &sampleDoc{Marker: marker})
}

func (p *sampleProbe) CountVisible(ctx context.Context, filter any) (int64, error) {
	docs, err := p.repo.Find(ctx, filter)
	if err != nil {
		return 0, err
	}
	return int64(len(docs)), nil
}

func (p *sampleProbe) UpdateSample(ctx context.Context, filter any) (int64, error) {
	res, err := p.repo.UpdateMany(ctx, filter, bson.M{"$set": bson.M{"marker": "tampered"}})
	if err != nil {
		return 0, err
	}
	return res.ModifiedCount, nil
}

func (p *sampleProbe) DeleteSample(ctx context.Context, filter any) (int64, error) {
	res, err := p.repo.DeleteMany(ctx, filter)
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

func (p *sampleProbe) AggregateCount(ctx context.Context) (int64, error) {
	res, err := p.repo.Aggregate(ctx, mongo.Pipeline{
		bson.D{{Key: "$count", Value: "total"}},
	})
	if err != nil {
		return 0, err
	}
	if len(res) == 0 {
		return 0, nil
	}
	switch v := res[0]["total"].(type) {
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	default:
		return 0, nil
	}
}

func init() {
	tenantdb.Register(tenantdb.Registration{
		Name:       "sample_records",
		Collection: "sample_records",
		NewProbe: func(client *mongo.Client) tenantdb.ScopedProbe {
			return &sampleProbe{repo: tenantdb.NewRepo[sampleDoc](client, "", "sample_records")}
		},
	})
}

func connectTestMongo(t *testing.T) *mongo.Client {
	t.Helper()
	ctx := context.Background()
	cl, err := mongodb.Connect(ctx, mongotest.URI(t))
	if err != nil {
		t.Fatalf("mongodb.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cl.Close(context.Background()) })
	return cl.Mongo()
}

func TestRepoIsolation(t *testing.T) {
	client := connectTestMongo(t)
	repo := tenantdb.NewRepo[sampleDoc](client, "", "isolation_items")
	ctx := context.Background()

	clinicA := platform.Clinic{ID: bson.NewObjectID(), Slug: "clinica-a", Tier: platform.TierStandard}
	clinicB := platform.Clinic{ID: bson.NewObjectID(), Slug: "clinica-b", Tier: platform.TierStandard}
	clinicC := platform.Clinic{ID: bson.NewObjectID(), Slug: "clinica-c", Tier: platform.TierDedicated}

	ctxA := platform.WithTenant(ctx, clinicA)
	ctxB := platform.WithTenant(ctx, clinicB)
	ctxC := platform.WithTenant(ctx, clinicC)

	t.Run("missing tenant fails without querying", func(t *testing.T) {
		if _, err := repo.Find(ctx, nil); !errors.Is(err, tenantdb.ErrNoTenant) {
			t.Fatalf("Find without tenant: got %v, want ErrNoTenant", err)
		}
		if _, err := repo.FindOne(ctx, nil); !errors.Is(err, tenantdb.ErrNoTenant) {
			t.Fatalf("FindOne without tenant: got %v, want ErrNoTenant", err)
		}
		if err := repo.Insert(ctx, &sampleDoc{Marker: "x"}); !errors.Is(err, tenantdb.ErrNoTenant) {
			t.Fatalf("Insert without tenant: got %v, want ErrNoTenant", err)
		}
		if _, err := repo.UpdateOne(ctx, nil, bson.M{"$set": bson.M{"marker": "y"}}); !errors.Is(err, tenantdb.ErrNoTenant) {
			t.Fatalf("UpdateOne without tenant: got %v, want ErrNoTenant", err)
		}
		if _, err := repo.UpdateMany(ctx, nil, bson.M{"$set": bson.M{"marker": "y"}}); !errors.Is(err, tenantdb.ErrNoTenant) {
			t.Fatalf("UpdateMany without tenant: got %v, want ErrNoTenant", err)
		}
		if _, err := repo.DeleteOne(ctx, nil); !errors.Is(err, tenantdb.ErrNoTenant) {
			t.Fatalf("DeleteOne without tenant: got %v, want ErrNoTenant", err)
		}
		if _, err := repo.DeleteMany(ctx, nil); !errors.Is(err, tenantdb.ErrNoTenant) {
			t.Fatalf("DeleteMany without tenant: got %v, want ErrNoTenant", err)
		}
		if _, err := repo.Aggregate(ctx, nil); !errors.Is(err, tenantdb.ErrNoTenant) {
			t.Fatalf("Aggregate without tenant: got %v, want ErrNoTenant", err)
		}
	})

	t.Run("insert rejects mismatched clinicId", func(t *testing.T) {
		err := repo.Insert(ctxA, &sampleDoc{ClinicID: clinicB.ID, Marker: "bad"})
		if !errors.Is(err, tenantdb.ErrClinicMismatch) {
			t.Fatalf("Insert with mismatched clinicId: got %v, want ErrClinicMismatch", err)
		}
	})

	if err := repo.Insert(ctxA, &sampleDoc{Marker: "a1"}); err != nil {
		t.Fatalf("Insert A: %v", err)
	}
	if err := repo.Insert(ctxB, &sampleDoc{Marker: "b1"}); err != nil {
		t.Fatalf("Insert B: %v", err)
	}
	if err := repo.Insert(ctxC, &sampleDoc{Marker: "c1"}); err != nil {
		t.Fatalf("Insert C (dedicated): %v", err)
	}

	t.Run("cross-tenant reads, filter injection, updates, deletes, and aggregations are blocked", func(t *testing.T) {
		docsA, err := repo.Find(ctxA, nil)
		if err != nil {
			t.Fatalf("Find A: %v", err)
		}
		if len(docsA) != 1 || docsA[0].Marker != "a1" || docsA[0].ClinicID != clinicA.ID {
			t.Fatalf("Find A unexpected docs: %+v", docsA)
		}

		injected, err := repo.Find(ctxA, bson.M{"clinicId": clinicB.ID})
		if err != nil {
			t.Fatalf("Find A with injected clinicId: %v", err)
		}
		if len(injected) != 0 {
			t.Fatalf("filter injection leaked %d docs from clinic B", len(injected))
		}

		if _, err := repo.FindOne(ctxA, bson.M{"marker": "b1"}); !errors.Is(err, mongo.ErrNoDocuments) {
			t.Fatalf("FindOne A for B's doc: got %v, want mongo.ErrNoDocuments", err)
		}

		updOne, err := repo.UpdateOne(ctxA, bson.M{"clinicId": clinicB.ID}, bson.M{"$set": bson.M{"marker": "pwn"}})
		if err != nil {
			t.Fatalf("UpdateOne A->B: %v", err)
		}
		if updOne.ModifiedCount != 0 {
			t.Fatalf("UpdateOne A->B modified %d docs", updOne.ModifiedCount)
		}

		updMany, err := repo.UpdateMany(ctxA, bson.M{"marker": "b1"}, bson.M{"$set": bson.M{"marker": "pwn"}})
		if err != nil {
			t.Fatalf("UpdateMany A->B: %v", err)
		}
		if updMany.ModifiedCount != 0 {
			t.Fatalf("UpdateMany A->B modified %d docs", updMany.ModifiedCount)
		}

		delOne, err := repo.DeleteOne(ctxA, bson.M{"clinicId": clinicB.ID})
		if err != nil {
			t.Fatalf("DeleteOne A->B: %v", err)
		}
		if delOne.DeletedCount != 0 {
			t.Fatalf("DeleteOne A->B deleted %d docs", delOne.DeletedCount)
		}

		delMany, err := repo.DeleteMany(ctxA, bson.M{"marker": "b1"})
		if err != nil {
			t.Fatalf("DeleteMany A->B: %v", err)
		}
		if delMany.DeletedCount != 0 {
			t.Fatalf("DeleteMany A->B deleted %d docs", delMany.DeletedCount)
		}

		aggA, err := repo.Aggregate(ctxA, mongo.Pipeline{
			bson.D{{Key: "$match", Value: bson.D{{Key: "marker", Value: "b1"}}}},
		})
		if err != nil {
			t.Fatalf("Aggregate A->B: %v", err)
		}
		if len(aggA) != 0 {
			t.Fatalf("Aggregate A->B leaked %d docs", len(aggA))
		}

		docsB, err := repo.Find(ctxB, nil)
		if err != nil {
			t.Fatalf("Find B: %v", err)
		}
		if len(docsB) != 1 || docsB[0].Marker != "b1" {
			t.Fatalf("Clinic B data was altered or missing: %+v", docsB)
		}

		docsC, err := repo.Find(ctxC, nil)
		if err != nil {
			t.Fatalf("Find C: %v", err)
		}
		if len(docsC) != 1 || docsC[0].Marker != "c1" {
			t.Fatalf("Clinic C dedicated data unexpected: %+v", docsC)
		}
		if got := repo.DatabaseName(clinicC); got != "clinic_"+clinicC.ID.Hex() {
			t.Fatalf("Dedicated DatabaseName: got %q", got)
		}
	})
}

func TestRegisteredRepositoriesIsolation(t *testing.T) {
	regs := tenantdb.Registered()
	if len(regs) == 0 {
		t.Fatal("expected at least one registered repository")
	}

	client := connectTestMongo(t)
	ctx := context.Background()

	for _, reg := range regs {
		t.Run(reg.Name, func(t *testing.T) {
			probe := reg.NewProbe(client)

			clinicA := platform.Clinic{ID: bson.NewObjectID(), Slug: "reg-a", Tier: platform.TierStandard}
			clinicB := platform.Clinic{ID: bson.NewObjectID(), Slug: "reg-b", Tier: platform.TierStandard}
			ctxA := platform.WithTenant(ctx, clinicA)
			ctxB := platform.WithTenant(ctx, clinicB)

			if _, err := probe.CountVisible(ctx, nil); !errors.Is(err, tenantdb.ErrNoTenant) {
				t.Fatalf("%s: CountVisible without tenant got %v, want ErrNoTenant", reg.Name, err)
			}

			if err := probe.InsertSample(ctxA, "only-a"); err != nil {
				t.Fatalf("%s: InsertSample A: %v", reg.Name, err)
			}
			if err := probe.InsertSample(ctxB, "only-b"); err != nil {
				t.Fatalf("%s: InsertSample B: %v", reg.Name, err)
			}

			if n, err := probe.CountVisible(ctxA, bson.M{"marker": "only-b"}); err != nil || n != 0 {
				t.Fatalf("%s: clinic A saw clinic B data (n=%d, err=%v)", reg.Name, n, err)
			}
			if n, err := probe.CountVisible(ctxA, bson.M{"clinicId": clinicB.ID}); err != nil || n != 0 {
				t.Fatalf("%s: clinic A bypassed filter with clinicId B (n=%d, err=%v)", reg.Name, n, err)
			}
			if n, err := probe.UpdateSample(ctxA, bson.M{"marker": "only-b"}); err != nil || n != 0 {
				t.Fatalf("%s: clinic A updated clinic B data (n=%d, err=%v)", reg.Name, n, err)
			}
			if n, err := probe.DeleteSample(ctxA, bson.M{"marker": "only-b"}); err != nil || n != 0 {
				t.Fatalf("%s: clinic A deleted clinic B data (n=%d, err=%v)", reg.Name, n, err)
			}
			if n, err := probe.AggregateCount(ctxA); err != nil || n != 1 {
				t.Fatalf("%s: clinic A aggregate count got %d (want 1), err=%v", reg.Name, n, err)
			}
		})
	}
}

func TestAllRepositoriesRegistered(t *testing.T) {
	registered := make(map[string]bool)
	for _, r := range tenantdb.Registered() {
		registered[r.Collection] = true
	}

	internalRoot := filepath.Clean("..")
	fset := token.NewFileSet()

	err := filepath.WalkDir(internalRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if filepath.Base(filepath.Dir(path)) == "tenantdb" {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if !isNewRepoCall(call.Fun) || len(call.Args) < 3 {
				return true
			}
			lit, ok := call.Args[2].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				t.Errorf("%s: tenantdb.NewRepo collection argument must be a string literal so registration can be verified", path)
				return true
			}
			col := strings.Trim(lit.Value, `"`)
			if !registered[col] {
				t.Errorf("%s: repository for collection %q is not registered via tenantdb.Register", path, col)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir %s: %v", internalRoot, err)
	}
}

func isNewRepoCall(expr ast.Expr) bool {
	if idx, ok := expr.(*ast.IndexExpr); ok {
		expr = idx.X
	}
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == "tenantdb" && sel.Sel.Name == "NewRepo"
}
