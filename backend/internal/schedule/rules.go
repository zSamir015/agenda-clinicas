// Package schedule manages clinic practitioners, rooms, services, weekly
// availability rules, time blocks, and Panamanian holiday preloading.
package schedule

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/zSamir015/agenda-clinicas/backend/internal/rbac"
	"github.com/zSamir015/agenda-clinicas/backend/internal/tenantdb"
)

const (
	collectionAvailabilityRules = "availability_rules"
	collectionBlocks            = "blocks"

	minutesPerDay = 24 * 60
	gridStepMins  = 5
)

// BlockScope defines what a block applies to within a clinic.
type BlockScope string

// Supported block scopes.
const (
	BlockScopeClinic       BlockScope = "clinic"
	BlockScopePractitioner BlockScope = "practitioner"
	BlockScopeRoom         BlockScope = "room"
)

// BlockSource records whether a block came from manual entry or holiday sync.
type BlockSource string

// Supported block sources.
const (
	BlockSourceManual  BlockSource = "manual"
	BlockSourceHoliday BlockSource = "holiday"
)

var (
	// ErrInvalidRule is returned when an availability rule fails domain validation.
	ErrInvalidRule = errors.New("schedule: invalid availability rule")
	// ErrRuleNotFound is returned when an availability rule does not exist in the clinic.
	ErrRuleNotFound = errors.New("schedule: availability rule not found")
	// ErrInvalidBlock is returned when a block fails domain validation.
	ErrInvalidBlock = errors.New("schedule: invalid block")
	// ErrBlockNotFound is returned when a block does not exist in the clinic.
	ErrBlockNotFound = errors.New("schedule: block not found")
)

// AvailabilityRule defines a recurring weekly working window for a practitioner and room.
type AvailabilityRule struct {
	ID             bson.ObjectID `bson:"_id,omitempty"`
	ClinicID       bson.ObjectID `bson:"clinicId"`
	PractitionerID bson.ObjectID `bson:"practitionerId"`
	RoomID         bson.ObjectID `bson:"roomId"`
	Weekday        time.Weekday  `bson:"weekday"`
	StartMinute    int           `bson:"startMinute"`
	EndMinute      int           `bson:"endMinute"`
	ValidFrom      *time.Time    `bson:"validFrom,omitempty"`
	ValidUntil     *time.Time    `bson:"validUntil,omitempty"`
	Active         bool          `bson:"active"`
	CreatedAt      time.Time     `bson:"createdAt"`
	UpdatedAt      time.Time     `bson:"updatedAt"`
}

// Validate checks domain invariants for an AvailabilityRule.
func (r AvailabilityRule) Validate() error {
	if r.PractitionerID.IsZero() {
		return fmt.Errorf("%w: practitionerId is required", ErrInvalidRule)
	}
	if r.RoomID.IsZero() {
		return fmt.Errorf("%w: roomId is required", ErrInvalidRule)
	}
	if r.Weekday < time.Sunday || r.Weekday > time.Saturday {
		return fmt.Errorf("%w: weekday must be 0..6", ErrInvalidRule)
	}
	if r.StartMinute < 0 || r.StartMinute >= minutesPerDay || r.StartMinute%gridStepMins != 0 {
		return fmt.Errorf("%w: startMinute must be a 5-minute multiple in [0, 1435]", ErrInvalidRule)
	}
	if r.EndMinute <= r.StartMinute || r.EndMinute > minutesPerDay || r.EndMinute%gridStepMins != 0 {
		return fmt.Errorf("%w: endMinute must be a 5-minute multiple > startMinute and <= 1440", ErrInvalidRule)
	}
	if r.ValidFrom != nil && r.ValidUntil != nil && r.ValidUntil.Before(*r.ValidFrom) {
		return fmt.Errorf("%w: validUntil must not precede validFrom", ErrInvalidRule)
	}
	return nil
}

// Block prevents bookings across a clinic, practitioner, or room for [StartAt, EndAt).
type Block struct {
	ID             bson.ObjectID  `bson:"_id,omitempty"`
	ClinicID       bson.ObjectID  `bson:"clinicId"`
	Scope          BlockScope     `bson:"scope"`
	PractitionerID *bson.ObjectID `bson:"practitionerId,omitempty"`
	RoomID         *bson.ObjectID `bson:"roomId,omitempty"`
	Reason         string         `bson:"reason"`
	Source         BlockSource    `bson:"source"`
	StartAt        time.Time      `bson:"startAt"`
	EndAt          time.Time      `bson:"endAt"`
	CreatedAt      time.Time      `bson:"createdAt"`
}

// Validate checks domain invariants for a Block.
func (b Block) Validate() error {
	if strings.TrimSpace(b.Reason) == "" {
		return fmt.Errorf("%w: reason is required", ErrInvalidBlock)
	}
	switch b.Scope {
	case BlockScopeClinic:
		if b.PractitionerID != nil || b.RoomID != nil {
			return fmt.Errorf("%w: clinic block must not target a practitioner or room", ErrInvalidBlock)
		}
	case BlockScopePractitioner:
		if b.PractitionerID == nil || b.PractitionerID.IsZero() || b.RoomID != nil {
			return fmt.Errorf("%w: practitioner block requires practitionerId and no roomId", ErrInvalidBlock)
		}
	case BlockScopeRoom:
		if b.RoomID == nil || b.RoomID.IsZero() || b.PractitionerID != nil {
			return fmt.Errorf("%w: room block requires roomId and no practitionerId", ErrInvalidBlock)
		}
	default:
		return fmt.Errorf("%w: unknown scope %q", ErrInvalidBlock, b.Scope)
	}
	switch b.Source {
	case BlockSourceManual, BlockSourceHoliday:
	default:
		return fmt.Errorf("%w: unknown source %q", ErrInvalidBlock, b.Source)
	}
	if !b.EndAt.After(b.StartAt) {
		return fmt.Errorf("%w: endAt must be after startAt", ErrInvalidBlock)
	}
	return nil
}

// RuleFilter narrows availability rule listings within the context clinic.
type RuleFilter struct {
	PractitionerID *bson.ObjectID
	RoomID         *bson.ObjectID
	ActiveOnly     bool
}

// RuleRepository stores availability rules scoped to the context clinic.
type RuleRepository struct {
	repo *tenantdb.Repo[AvailabilityRule]
	now  func() time.Time
}

// NewRuleRepository returns a RuleRepository backed by client.
func NewRuleRepository(client *mongo.Client, sharedDB string) *RuleRepository {
	return &RuleRepository{
		repo: tenantdb.NewRepo[AvailabilityRule](client, sharedDB, "availability_rules"),
		now:  func() time.Time { return time.Now().UTC().Truncate(time.Millisecond) },
	}
}

// Create validates and inserts an AvailabilityRule for the context clinic.
func (r *RuleRepository) Create(ctx context.Context, rule AvailabilityRule) (AvailabilityRule, error) {
	if err := rule.Validate(); err != nil {
		return AvailabilityRule{}, err
	}
	now := r.now()
	if rule.ID.IsZero() {
		rule.ID = bson.NewObjectID()
	}
	rule.CreatedAt = now
	rule.UpdatedAt = now
	rule.ValidFrom = normalizeTimePtr(rule.ValidFrom)
	rule.ValidUntil = normalizeTimePtr(rule.ValidUntil)

	if err := r.repo.Insert(ctx, &rule); err != nil {
		return AvailabilityRule{}, err
	}
	return r.ByID(ctx, rule.ID)
}

// ByID returns the rule identified by id in the context clinic.
func (r *RuleRepository) ByID(ctx context.Context, id bson.ObjectID) (AvailabilityRule, error) {
	got, err := r.repo.FindOne(ctx, bson.D{{Key: "_id", Value: id}})
	if errors.Is(err, mongo.ErrNoDocuments) {
		return AvailabilityRule{}, ErrRuleNotFound
	}
	if err != nil {
		return AvailabilityRule{}, err
	}
	return got, nil
}

// List returns matching rules ordered by weekday and startMinute.
func (r *RuleRepository) List(ctx context.Context, f RuleFilter) ([]AvailabilityRule, error) {
	filter := bson.D{}
	if f.PractitionerID != nil && !f.PractitionerID.IsZero() {
		filter = append(filter, bson.E{Key: "practitionerId", Value: *f.PractitionerID})
	}
	if f.RoomID != nil && !f.RoomID.IsZero() {
		filter = append(filter, bson.E{Key: "roomId", Value: *f.RoomID})
	}
	if f.ActiveOnly {
		filter = append(filter, bson.E{Key: "active", Value: true})
	}
	opts := options.Find().SetSort(bson.D{{Key: "weekday", Value: 1}, {Key: "startMinute", Value: 1}})
	return r.repo.Find(ctx, filter, opts)
}

// Update validates and replaces the editable fields of an existing rule.
func (r *RuleRepository) Update(ctx context.Context, rule AvailabilityRule) (AvailabilityRule, error) {
	if rule.ID.IsZero() {
		return AvailabilityRule{}, ErrRuleNotFound
	}
	if err := rule.Validate(); err != nil {
		return AvailabilityRule{}, err
	}

	now := r.now()
	set := bson.D{
		{Key: "practitionerId", Value: rule.PractitionerID},
		{Key: "roomId", Value: rule.RoomID},
		{Key: "weekday", Value: rule.Weekday},
		{Key: "startMinute", Value: rule.StartMinute},
		{Key: "endMinute", Value: rule.EndMinute},
		{Key: "active", Value: rule.Active},
		{Key: "updatedAt", Value: now},
	}
	unset := bson.D{}
	if vf := normalizeTimePtr(rule.ValidFrom); vf != nil {
		set = append(set, bson.E{Key: "validFrom", Value: *vf})
	} else {
		unset = append(unset, bson.E{Key: "validFrom", Value: ""})
	}
	if vu := normalizeTimePtr(rule.ValidUntil); vu != nil {
		set = append(set, bson.E{Key: "validUntil", Value: *vu})
	} else {
		unset = append(unset, bson.E{Key: "validUntil", Value: ""})
	}

	updateDoc := bson.D{{Key: "$set", Value: set}}
	if len(unset) > 0 {
		updateDoc = append(updateDoc, bson.E{Key: "$unset", Value: unset})
	}

	res, err := r.repo.UpdateOne(ctx, bson.D{{Key: "_id", Value: rule.ID}}, updateDoc)
	if err != nil {
		return AvailabilityRule{}, err
	}
	if res.MatchedCount == 0 {
		return AvailabilityRule{}, ErrRuleNotFound
	}
	return r.ByID(ctx, rule.ID)
}

// Delete removes an availability rule by ID within the context clinic.
func (r *RuleRepository) Delete(ctx context.Context, id bson.ObjectID) error {
	res, err := r.repo.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrRuleNotFound
	}
	return nil
}

// BlockRepository stores clinic, practitioner, and room blocks scoped to the context clinic.
type BlockRepository struct {
	repo *tenantdb.Repo[Block]
	now  func() time.Time
}

// NewBlockRepository returns a BlockRepository backed by client.
func NewBlockRepository(client *mongo.Client, sharedDB string) *BlockRepository {
	return &BlockRepository{
		repo: tenantdb.NewRepo[Block](client, sharedDB, "blocks"),
		now:  func() time.Time { return time.Now().UTC().Truncate(time.Millisecond) },
	}
}

// Create validates and inserts a Block for the context clinic.
func (r *BlockRepository) Create(ctx context.Context, b Block) (Block, error) {
	b.Reason = strings.TrimSpace(b.Reason)
	if b.Source == "" {
		b.Source = BlockSourceManual
	}
	b.StartAt = b.StartAt.UTC().Truncate(time.Millisecond)
	b.EndAt = b.EndAt.UTC().Truncate(time.Millisecond)
	if err := b.Validate(); err != nil {
		return Block{}, err
	}

	if b.ID.IsZero() {
		b.ID = bson.NewObjectID()
	}
	b.CreatedAt = r.now()
	if err := r.repo.Insert(ctx, &b); err != nil {
		return Block{}, err
	}
	return r.ByID(ctx, b.ID)
}

// ByID returns the block matching id in the context clinic.
func (r *BlockRepository) ByID(ctx context.Context, id bson.ObjectID) (Block, error) {
	got, err := r.repo.FindOne(ctx, bson.D{{Key: "_id", Value: id}})
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Block{}, ErrBlockNotFound
	}
	if err != nil {
		return Block{}, err
	}
	return got, nil
}

// ListInRange returns blocks in the context clinic overlapping [startAt, endAt).
func (r *BlockRepository) ListInRange(ctx context.Context, startAt, endAt time.Time) ([]Block, error) {
	if !endAt.After(startAt) {
		return nil, fmt.Errorf("%w: query endAt must be after startAt", ErrInvalidBlock)
	}
	filter := bson.D{
		{Key: "startAt", Value: bson.D{{Key: "$lt", Value: endAt.UTC()}}},
		{Key: "endAt", Value: bson.D{{Key: "$gt", Value: startAt.UTC()}}},
	}
	opts := options.Find().SetSort(bson.D{{Key: "startAt", Value: 1}})
	return r.repo.Find(ctx, filter, opts)
}

// Delete removes a block by ID within the context clinic.
func (r *BlockRepository) Delete(ctx context.Context, id bson.ObjectID) error {
	res, err := r.repo.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrBlockNotFound
	}
	return nil
}

// RuleService enforces RBAC permissions (schedule.manage own/all) over rules and blocks.
type RuleService struct {
	rules  *RuleRepository
	blocks *BlockRepository
}

// NewRuleService builds a RuleService over rule and block repositories.
func NewRuleService(rules *RuleRepository, blocks *BlockRepository) *RuleService {
	return &RuleService{rules: rules, blocks: blocks}
}

// CreateRule authorizes the caller and creates an availability rule.
func (s *RuleService) CreateRule(ctx context.Context, grants []rbac.Grant, actorPractitionerID *bson.ObjectID, rule AvailabilityRule) (AvailabilityRule, error) {
	if err := authorizePractitionerManage(grants, actorPractitionerID, rule.PractitionerID); err != nil {
		return AvailabilityRule{}, err
	}
	return s.rules.Create(ctx, rule)
}

// UpdateRule authorizes the caller and updates an existing availability rule.
func (s *RuleService) UpdateRule(ctx context.Context, grants []rbac.Grant, actorPractitionerID *bson.ObjectID, rule AvailabilityRule) (AvailabilityRule, error) {
	existing, err := s.rules.ByID(ctx, rule.ID)
	if err != nil {
		return AvailabilityRule{}, err
	}
	if err := authorizePractitionerManage(grants, actorPractitionerID, existing.PractitionerID); err != nil {
		return AvailabilityRule{}, err
	}
	if err := authorizePractitionerManage(grants, actorPractitionerID, rule.PractitionerID); err != nil {
		return AvailabilityRule{}, err
	}
	return s.rules.Update(ctx, rule)
}

// DeleteRule authorizes the caller and deletes an availability rule.
func (s *RuleService) DeleteRule(ctx context.Context, grants []rbac.Grant, actorPractitionerID *bson.ObjectID, ruleID bson.ObjectID) error {
	existing, err := s.rules.ByID(ctx, ruleID)
	if err != nil {
		return err
	}
	if err := authorizePractitionerManage(grants, actorPractitionerID, existing.PractitionerID); err != nil {
		return err
	}
	return s.rules.Delete(ctx, ruleID)
}

// ListRules authorizes reading rules (own or all) and lists matching rules.
func (s *RuleService) ListRules(ctx context.Context, grants []rbac.Grant, actorPractitionerID *bson.ObjectID, f RuleFilter) ([]AvailabilityRule, error) {
	if err := rbac.Authorize(grants, rbac.PermScheduleManage, rbac.ScopeAll); err == nil {
		return s.rules.List(ctx, f)
	}
	if err := rbac.Authorize(grants, rbac.PermScheduleManage, rbac.ScopeOwn); err != nil {
		return nil, err
	}
	if actorPractitionerID == nil || actorPractitionerID.IsZero() {
		return nil, rbac.ErrForbidden
	}
	if f.PractitionerID != nil && *f.PractitionerID != *actorPractitionerID {
		return nil, rbac.ErrForbidden
	}
	f.PractitionerID = actorPractitionerID
	return s.rules.List(ctx, f)
}

// CreateBlock authorizes the caller and creates a schedule block.
func (s *RuleService) CreateBlock(ctx context.Context, grants []rbac.Grant, actorPractitionerID *bson.ObjectID, block Block) (Block, error) {
	if block.Scope == BlockScopePractitioner && block.PractitionerID != nil {
		if err := authorizePractitionerManage(grants, actorPractitionerID, *block.PractitionerID); err != nil {
			return Block{}, err
		}
	} else {
		if err := rbac.Authorize(grants, rbac.PermScheduleManage, rbac.ScopeAll); err != nil {
			return Block{}, err
		}
	}
	return s.blocks.Create(ctx, block)
}

// DeleteBlock authorizes the caller and deletes a block.
func (s *RuleService) DeleteBlock(ctx context.Context, grants []rbac.Grant, actorPractitionerID *bson.ObjectID, blockID bson.ObjectID) error {
	existing, err := s.blocks.ByID(ctx, blockID)
	if err != nil {
		return err
	}
	if existing.Scope == BlockScopePractitioner && existing.PractitionerID != nil {
		if err := authorizePractitionerManage(grants, actorPractitionerID, *existing.PractitionerID); err != nil {
			return err
		}
	} else {
		if err := rbac.Authorize(grants, rbac.PermScheduleManage, rbac.ScopeAll); err != nil {
			return err
		}
	}
	return s.blocks.Delete(ctx, blockID)
}

func authorizePractitionerManage(grants []rbac.Grant, actorPractitionerID *bson.ObjectID, targetPractitionerID bson.ObjectID) error {
	if err := rbac.Authorize(grants, rbac.PermScheduleManage, rbac.ScopeAll); err == nil {
		return nil
	}
	if err := rbac.Authorize(grants, rbac.PermScheduleManage, rbac.ScopeOwn); err != nil {
		return err
	}
	if actorPractitionerID == nil || actorPractitionerID.IsZero() || *actorPractitionerID != targetPractitionerID {
		return rbac.ErrForbidden
	}
	return nil
}

func normalizeTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := t.UTC().Truncate(time.Millisecond)
	return &v
}

// Register schedule rule/block repositories with tenantdb isolation checks.
func init() {
	tenantdb.Register(tenantdb.Registration{
		Name:       collectionAvailabilityRules,
		Collection: collectionAvailabilityRules,
		NewProbe: func(client *mongo.Client) tenantdb.ScopedProbe {
			return &collectionProbe{repo: tenantdb.NewRepo[probeDoc](client, "", "availability_rules")}
		},
	})
	tenantdb.Register(tenantdb.Registration{
		Name:       collectionBlocks,
		Collection: collectionBlocks,
		NewProbe: func(client *mongo.Client) tenantdb.ScopedProbe {
			return &collectionProbe{repo: tenantdb.NewRepo[probeDoc](client, "", "blocks")}
		},
	})
}

type probeDoc struct {
	ID       bson.ObjectID `bson:"_id,omitempty"`
	ClinicID bson.ObjectID `bson:"clinicId,omitempty"`
	Marker   string        `bson:"marker"`
}

type collectionProbe struct {
	repo *tenantdb.Repo[probeDoc]
}

func (p *collectionProbe) InsertSample(ctx context.Context, marker string) error {
	return p.repo.Insert(ctx, &probeDoc{Marker: marker})
}

func (p *collectionProbe) CountVisible(ctx context.Context, filter any) (int64, error) {
	docs, err := p.repo.Find(ctx, filter)
	if err != nil {
		return 0, err
	}
	return int64(len(docs)), nil
}

func (p *collectionProbe) UpdateSample(ctx context.Context, filter any) (int64, error) {
	res, err := p.repo.UpdateMany(ctx, filter, bson.M{"$set": bson.M{"marker": "tampered"}})
	if err != nil {
		return 0, err
	}
	return res.ModifiedCount, nil
}

func (p *collectionProbe) DeleteSample(ctx context.Context, filter any) (int64, error) {
	res, err := p.repo.DeleteMany(ctx, filter)
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

func (p *collectionProbe) AggregateCount(ctx context.Context) (int64, error) {
	res, err := p.repo.Aggregate(ctx, mongo.Pipeline{bson.D{{Key: "$count", Value: "total"}}})
	if err != nil || len(res) == 0 {
		return 0, err
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
