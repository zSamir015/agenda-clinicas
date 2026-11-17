package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Holiday describes an official or executive-decree Panamanian holiday entry
// from data/panama-holidays.json.
type Holiday struct {
	Date              string `json:"date"`
	OriginalDate      string `json:"originalDate,omitempty"`
	Name              string `json:"name"`
	Movable           bool   `json:"movable"`
	ShiftedFromSunday bool   `json:"shiftedFromSunday"`
	Confirmed         bool   `json:"confirmed"`
	RequiresDecree    bool   `json:"requiresDecree"`
	LegalSource       string `json:"legalSource"`
}

type holidayCatalog struct {
	Country  string    `json:"country"`
	Timezone string    `json:"timezone"`
	Years    []int     `json:"years"`
	Holidays []Holiday `json:"holidays"`
}

// HolidaySyncOptions controls which holiday entries are preloaded into a clinic.
type HolidaySyncOptions struct {
	// IncludeUnconfirmed also seeds holidays subject to annual executive decree
	// (requiresDecree: true, confirmed: false). When false, only statutory
	// holidays with confirmed: true are preloaded.
	IncludeUnconfirmed bool
}

// PanamaLocation returns the America/Panama location (UTC-5 fixed).
func PanamaLocation() *time.Location {
	loc, err := time.LoadLocation("America/Panama")
	if err != nil {
		return time.FixedZone("America/Panama", -5*60*60)
	}
	return loc
}

// ParsePanamaHolidays decodes holidays JSON from r.
func ParsePanamaHolidays(r io.Reader) ([]Holiday, error) {
	var cat holidayCatalog
	if err := json.NewDecoder(r).Decode(&cat); err != nil {
		return nil, fmt.Errorf("decode panama holidays: %w", err)
	}
	if len(cat.Holidays) == 0 {
		return nil, errors.New("schedule: empty panama holidays catalog")
	}
	return cat.Holidays, nil
}

// LoadPanamaHolidays reads and parses holidays from path.
func LoadPanamaHolidays(path string) ([]Holiday, error) {
	f, err := os.Open(path) //nolint:gosec // controlled path to static data file
	if err != nil {
		return nil, fmt.Errorf("open panama holidays %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	return ParsePanamaHolidays(f)
}

// LoadDefaultPanamaHolidays resolves data/panama-holidays.json from
// PANAMA_HOLIDAYS_PATH or parent directories up to the repository root.
func LoadDefaultPanamaHolidays() ([]Holiday, error) {
	if envPath := os.Getenv("PANAMA_HOLIDAYS_PATH"); envPath != "" {
		return LoadPanamaHolidays(envPath)
	}
	candidates := []string{
		filepath.Join("data", "panama-holidays.json"),
		filepath.Join("..", "data", "panama-holidays.json"),
		filepath.Join("..", "..", "data", "panama-holidays.json"),
		filepath.Join("..", "..", "..", "data", "panama-holidays.json"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return LoadPanamaHolidays(p)
		}
	}
	return nil, errors.New("schedule: data/panama-holidays.json not found")
}

// HolidayWindowUTC converts a YYYY-MM-DD Panama calendar day into [startUTC, endUTC).
func HolidayWindowUTC(dateStr string) (time.Time, time.Time, error) {
	loc := PanamaLocation()
	localStart, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid holiday date %q: %w", dateStr, err)
	}
	startUTC := localStart.UTC()
	endUTC := localStart.Add(24 * time.Hour).UTC()
	return startUTC, endUTC, nil
}

// SyncPanamaHolidays idempotently creates clinic-wide holiday blocks in the
// context clinic for each selected holiday, returning the count inserted.
func SyncPanamaHolidays(ctx context.Context, blocks *BlockRepository, holidays []Holiday, opts HolidaySyncOptions) (int, error) {
	if blocks == nil {
		return 0, errors.New("schedule: block repository is required")
	}

	existing, err := blocks.repo.Find(ctx, bson.D{
		{Key: "scope", Value: BlockScopeClinic},
		{Key: "source", Value: BlockSourceHoliday},
	})
	if err != nil {
		return 0, fmt.Errorf("list existing holiday blocks: %w", err)
	}

	type holidayKey struct {
		startUnix int64
		reason    string
	}
	seen := make(map[holidayKey]struct{}, len(existing))
	for _, b := range existing {
		seen[holidayKey{startUnix: b.StartAt.UTC().Unix(), reason: b.Reason}] = struct{}{}
	}

	inserted := 0
	for _, h := range holidays {
		if !h.Confirmed && !opts.IncludeUnconfirmed {
			continue
		}
		startAt, endAt, err := HolidayWindowUTC(h.Date)
		if err != nil {
			return inserted, err
		}
		key := holidayKey{startUnix: startAt.Unix(), reason: h.Name}
		if _, exists := seen[key]; exists {
			continue
		}
		if _, err := blocks.Create(ctx, Block{
			Scope:   BlockScopeClinic,
			Reason:  h.Name,
			Source:  BlockSourceHoliday,
			StartAt: startAt,
			EndAt:   endAt,
		}); err != nil {
			return inserted, fmt.Errorf("insert holiday block %s (%s): %w", h.Date, h.Name, err)
		}
		seen[key] = struct{}{}
		inserted++
	}
	return inserted, nil
}

// SyncDefaultPanamaHolidays loads data/panama-holidays.json and idempotently
// preloads holiday blocks into the context clinic.
func SyncDefaultPanamaHolidays(ctx context.Context, client *mongo.Client, opts HolidaySyncOptions) (int, error) {
	holidays, err := LoadDefaultPanamaHolidays()
	if err != nil {
		return 0, err
	}
	return SyncPanamaHolidays(ctx, NewBlockRepository(client, ""), holidays, opts)
}
