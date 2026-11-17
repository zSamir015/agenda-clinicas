package schedule

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/zSamir015/agenda-clinicas/backend/internal/rbac"
	"github.com/zSamir015/agenda-clinicas/backend/internal/tenantdb"
)

func utcNow() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }

// activeFilter returns the filter for listings that can hide inactive records.
func activeFilter(activeOnly bool) bson.D {
	if activeOnly {
		return bson.D{{Key: "active", Value: true}}
	}
	return bson.D{}
}

// ---- practitioners ----

// PractitionerRepository stores practitioners scoped to the context clinic.
type PractitionerRepository struct {
	repo *tenantdb.Repo[Practitioner]
	now  func() time.Time
}

// NewPractitionerRepository returns a repository backed by client.
func NewPractitionerRepository(client *mongo.Client, sharedDB string) *PractitionerRepository {
	return &PractitionerRepository{
		repo: tenantdb.NewRepo[Practitioner](client, sharedDB, "practitioners"),
		now:  utcNow,
	}
}

// Create validates and inserts p for the context clinic.
func (r *PractitionerRepository) Create(ctx context.Context, p Practitioner) (Practitioner, error) {
	p.normalize()
	if err := p.Validate(); err != nil {
		return Practitioner{}, err
	}
	if p.ID.IsZero() {
		p.ID = bson.NewObjectID()
	}
	now := r.now()
	p.CreatedAt, p.UpdatedAt = now, now
	if err := r.repo.Insert(ctx, &p); err != nil {
		return Practitioner{}, err
	}
	return r.ByID(ctx, p.ID)
}

// ByID returns the practitioner identified by id in the context clinic.
func (r *PractitionerRepository) ByID(ctx context.Context, id bson.ObjectID) (Practitioner, error) {
	got, err := r.repo.FindOne(ctx, bson.D{{Key: "_id", Value: id}})
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Practitioner{}, ErrPractitionerNotFound
	}
	return got, err
}

// List returns practitioners ordered by name.
func (r *PractitionerRepository) List(ctx context.Context, activeOnly bool) ([]Practitioner, error) {
	return r.repo.Find(ctx, activeFilter(activeOnly), options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
}

// Update replaces the mutable fields of p.
func (r *PractitionerRepository) Update(ctx context.Context, p Practitioner) (Practitioner, error) {
	if p.ID.IsZero() {
		return Practitioner{}, ErrPractitionerNotFound
	}
	p.normalize()
	if err := p.Validate(); err != nil {
		return Practitioner{}, err
	}
	set := bson.D{
		{Key: "name", Value: p.Name},
		{Key: "specialty", Value: p.Specialty},
		{Key: "serviceIds", Value: p.ServiceIDs},
		{Key: "active", Value: p.Active},
		{Key: "updatedAt", Value: r.now()},
	}
	var update bson.D
	if p.UserID != nil {
		set = append(set, bson.E{Key: "userId", Value: *p.UserID})
		update = bson.D{{Key: "$set", Value: set}}
	} else {
		update = bson.D{{Key: "$set", Value: set}, {Key: "$unset", Value: bson.D{{Key: "userId", Value: ""}}}}
	}
	res, err := r.repo.UpdateOne(ctx, bson.D{{Key: "_id", Value: p.ID}}, update)
	if err != nil {
		return Practitioner{}, err
	}
	if res.MatchedCount == 0 {
		return Practitioner{}, ErrPractitionerNotFound
	}
	return r.ByID(ctx, p.ID)
}

// ---- rooms ----

// RoomRepository stores rooms scoped to the context clinic.
type RoomRepository struct {
	repo *tenantdb.Repo[Room]
	now  func() time.Time
}

// NewRoomRepository returns a repository backed by client.
func NewRoomRepository(client *mongo.Client, sharedDB string) *RoomRepository {
	return &RoomRepository{
		repo: tenantdb.NewRepo[Room](client, sharedDB, "rooms"),
		now:  utcNow,
	}
}

// Create validates and inserts rm for the context clinic.
func (r *RoomRepository) Create(ctx context.Context, rm Room) (Room, error) {
	rm.normalize()
	if err := rm.Validate(); err != nil {
		return Room{}, err
	}
	if rm.ID.IsZero() {
		rm.ID = bson.NewObjectID()
	}
	now := r.now()
	rm.CreatedAt, rm.UpdatedAt = now, now
	if err := r.repo.Insert(ctx, &rm); err != nil {
		return Room{}, mapDuplicate(err)
	}
	return r.ByID(ctx, rm.ID)
}

// ByID returns the room identified by id in the context clinic.
func (r *RoomRepository) ByID(ctx context.Context, id bson.ObjectID) (Room, error) {
	got, err := r.repo.FindOne(ctx, bson.D{{Key: "_id", Value: id}})
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Room{}, ErrRoomNotFound
	}
	return got, err
}

// List returns rooms ordered by name.
func (r *RoomRepository) List(ctx context.Context, activeOnly bool) ([]Room, error) {
	return r.repo.Find(ctx, activeFilter(activeOnly), options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
}

// Update replaces the mutable fields of rm.
func (r *RoomRepository) Update(ctx context.Context, rm Room) (Room, error) {
	if rm.ID.IsZero() {
		return Room{}, ErrRoomNotFound
	}
	rm.normalize()
	if err := rm.Validate(); err != nil {
		return Room{}, err
	}
	res, err := r.repo.UpdateOne(ctx, bson.D{{Key: "_id", Value: rm.ID}}, bson.D{{Key: "$set", Value: bson.D{
		{Key: "name", Value: rm.Name},
		{Key: "roomType", Value: rm.RoomType},
		{Key: "active", Value: rm.Active},
		{Key: "updatedAt", Value: r.now()},
	}}})
	if err != nil {
		return Room{}, mapDuplicate(err)
	}
	if res.MatchedCount == 0 {
		return Room{}, ErrRoomNotFound
	}
	return r.ByID(ctx, rm.ID)
}

// ---- services ----

// ServiceRepository stores services scoped to the context clinic.
type ServiceRepository struct {
	repo *tenantdb.Repo[Service]
	now  func() time.Time
}

// NewServiceRepository returns a repository backed by client.
func NewServiceRepository(client *mongo.Client, sharedDB string) *ServiceRepository {
	return &ServiceRepository{
		repo: tenantdb.NewRepo[Service](client, sharedDB, "services"),
		now:  utcNow,
	}
}

// Create validates and inserts s for the context clinic.
func (r *ServiceRepository) Create(ctx context.Context, s Service) (Service, error) {
	s.normalize()
	if err := s.Validate(); err != nil {
		return Service{}, err
	}
	if s.ID.IsZero() {
		s.ID = bson.NewObjectID()
	}
	now := r.now()
	s.CreatedAt, s.UpdatedAt = now, now
	if err := r.repo.Insert(ctx, &s); err != nil {
		return Service{}, mapDuplicate(err)
	}
	return r.ByID(ctx, s.ID)
}

// ByID returns the service identified by id in the context clinic.
func (r *ServiceRepository) ByID(ctx context.Context, id bson.ObjectID) (Service, error) {
	got, err := r.repo.FindOne(ctx, bson.D{{Key: "_id", Value: id}})
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Service{}, ErrServiceNotFound
	}
	return got, err
}

// List returns services ordered by name.
func (r *ServiceRepository) List(ctx context.Context, activeOnly bool) ([]Service, error) {
	return r.repo.Find(ctx, activeFilter(activeOnly), options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
}

// Update replaces the mutable fields of s.
func (r *ServiceRepository) Update(ctx context.Context, s Service) (Service, error) {
	if s.ID.IsZero() {
		return Service{}, ErrServiceNotFound
	}
	s.normalize()
	if err := s.Validate(); err != nil {
		return Service{}, err
	}
	res, err := r.repo.UpdateOne(ctx, bson.D{{Key: "_id", Value: s.ID}}, bson.D{{Key: "$set", Value: bson.D{
		{Key: "name", Value: s.Name},
		{Key: "durationMinutes", Value: s.DurationMinutes},
		{Key: "bufferMinutes", Value: s.BufferMinutes},
		{Key: "roomType", Value: s.RoomType},
		{Key: "paymentMode", Value: s.PaymentMode},
		{Key: "priceCents", Value: s.PriceCents},
		{Key: "depositCents", Value: s.DepositCents},
		{Key: "depositPercent", Value: s.DepositPercent},
		{Key: "active", Value: s.Active},
		{Key: "updatedAt", Value: r.now()},
	}}})
	if err != nil {
		return Service{}, mapDuplicate(err)
	}
	if res.MatchedCount == 0 {
		return Service{}, ErrServiceNotFound
	}
	return r.ByID(ctx, s.ID)
}

// existAll reports whether every id is a service of the context clinic.
func (r *ServiceRepository) existAll(ctx context.Context, ids []bson.ObjectID) error {
	if len(ids) == 0 {
		return nil
	}
	found, err := r.repo.Find(ctx, bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: ids}}}})
	if err != nil {
		return err
	}
	if len(found) != len(ids) {
		return fmt.Errorf("%w: a serviceId does not exist in this clinic", ErrInvalidPractitioner)
	}
	return nil
}

func mapDuplicate(err error) error {
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicateName
	}
	return err
}

// ---- service layer with authorization ----

// CatalogService applies RBAC on top of the catalog repositories.
//
// Reading the catalog needs schedule.manage (any scope) or clinic.settings,
// because reception books against it. Changing it needs clinic.settings on
// the whole clinic. Records are deactivated, not deleted: bookings and
// availability rules refer to them.
type CatalogService struct {
	practitioners *PractitionerRepository
	rooms         *RoomRepository
	services      *ServiceRepository
}

// NewCatalogService wires the three repositories.
func NewCatalogService(p *PractitionerRepository, r *RoomRepository, s *ServiceRepository) *CatalogService {
	return &CatalogService{practitioners: p, rooms: r, services: s}
}

func authorizeCatalogRead(grants []rbac.Grant) error {
	if err := rbac.Authorize(grants, rbac.PermScheduleManage, rbac.ScopeOwn); err == nil {
		return nil
	}
	return rbac.Authorize(grants, rbac.PermClinicSettings, rbac.ScopeAll)
}

func authorizeCatalogWrite(grants []rbac.Grant) error {
	return rbac.Authorize(grants, rbac.PermClinicSettings, rbac.ScopeAll)
}

// CreatePractitioner creates a practitioner whose services exist in the clinic.
func (s *CatalogService) CreatePractitioner(ctx context.Context, grants []rbac.Grant, p Practitioner) (Practitioner, error) {
	if err := authorizeCatalogWrite(grants); err != nil {
		return Practitioner{}, err
	}
	p.normalize()
	if err := p.Validate(); err != nil {
		return Practitioner{}, err
	}
	if err := s.services.existAll(ctx, p.ServiceIDs); err != nil {
		return Practitioner{}, err
	}
	return s.practitioners.Create(ctx, p)
}

// UpdatePractitioner updates a practitioner whose services exist in the clinic.
func (s *CatalogService) UpdatePractitioner(ctx context.Context, grants []rbac.Grant, p Practitioner) (Practitioner, error) {
	if err := authorizeCatalogWrite(grants); err != nil {
		return Practitioner{}, err
	}
	if p.ID.IsZero() {
		return Practitioner{}, ErrPractitionerNotFound
	}
	p.normalize()
	if err := p.Validate(); err != nil {
		return Practitioner{}, err
	}
	if err := s.services.existAll(ctx, p.ServiceIDs); err != nil {
		return Practitioner{}, err
	}
	return s.practitioners.Update(ctx, p)
}

// GetPractitioner returns a practitioner by ID.
func (s *CatalogService) GetPractitioner(ctx context.Context, grants []rbac.Grant, id bson.ObjectID) (Practitioner, error) {
	if err := authorizeCatalogRead(grants); err != nil {
		return Practitioner{}, err
	}
	return s.practitioners.ByID(ctx, id)
}

// ListPractitioners lists practitioners.
func (s *CatalogService) ListPractitioners(ctx context.Context, grants []rbac.Grant, activeOnly bool) ([]Practitioner, error) {
	if err := authorizeCatalogRead(grants); err != nil {
		return nil, err
	}
	return s.practitioners.List(ctx, activeOnly)
}

// CreateRoom creates a room.
func (s *CatalogService) CreateRoom(ctx context.Context, grants []rbac.Grant, rm Room) (Room, error) {
	if err := authorizeCatalogWrite(grants); err != nil {
		return Room{}, err
	}
	return s.rooms.Create(ctx, rm)
}

// UpdateRoom updates a room.
func (s *CatalogService) UpdateRoom(ctx context.Context, grants []rbac.Grant, rm Room) (Room, error) {
	if err := authorizeCatalogWrite(grants); err != nil {
		return Room{}, err
	}
	return s.rooms.Update(ctx, rm)
}

// GetRoom returns a room by ID.
func (s *CatalogService) GetRoom(ctx context.Context, grants []rbac.Grant, id bson.ObjectID) (Room, error) {
	if err := authorizeCatalogRead(grants); err != nil {
		return Room{}, err
	}
	return s.rooms.ByID(ctx, id)
}

// ListRooms lists rooms.
func (s *CatalogService) ListRooms(ctx context.Context, grants []rbac.Grant, activeOnly bool) ([]Room, error) {
	if err := authorizeCatalogRead(grants); err != nil {
		return nil, err
	}
	return s.rooms.List(ctx, activeOnly)
}

// CreateService creates a bookable service.
func (s *CatalogService) CreateService(ctx context.Context, grants []rbac.Grant, sv Service) (Service, error) {
	if err := authorizeCatalogWrite(grants); err != nil {
		return Service{}, err
	}
	return s.services.Create(ctx, sv)
}

// UpdateService updates a bookable service.
func (s *CatalogService) UpdateService(ctx context.Context, grants []rbac.Grant, sv Service) (Service, error) {
	if err := authorizeCatalogWrite(grants); err != nil {
		return Service{}, err
	}
	return s.services.Update(ctx, sv)
}

// GetService returns a service by ID.
func (s *CatalogService) GetService(ctx context.Context, grants []rbac.Grant, id bson.ObjectID) (Service, error) {
	if err := authorizeCatalogRead(grants); err != nil {
		return Service{}, err
	}
	return s.services.ByID(ctx, id)
}

// ListServices lists bookable services.
func (s *CatalogService) ListServices(ctx context.Context, grants []rbac.Grant, activeOnly bool) ([]Service, error) {
	if err := authorizeCatalogRead(grants); err != nil {
		return nil, err
	}
	return s.services.List(ctx, activeOnly)
}

// ---- tenant isolation registration ----

func init() {
	// The collection name must be a string literal in each NewRepo call so
	// tenantdb's registration check can verify it.
	tenantdb.Register(tenantdb.Registration{
		Name:       "practitioners",
		Collection: "practitioners",
		NewProbe: func(client *mongo.Client) tenantdb.ScopedProbe {
			return &catalogProbe{repo: tenantdb.NewRepo[catalogProbeDoc](client, "", "practitioners")}
		},
	})
	tenantdb.Register(tenantdb.Registration{
		Name:       "rooms",
		Collection: "rooms",
		NewProbe: func(client *mongo.Client) tenantdb.ScopedProbe {
			return &catalogProbe{repo: tenantdb.NewRepo[catalogProbeDoc](client, "", "rooms")}
		},
	})
	tenantdb.Register(tenantdb.Registration{
		Name:       "services",
		Collection: "services",
		NewProbe: func(client *mongo.Client) tenantdb.ScopedProbe {
			return &catalogProbe{repo: tenantdb.NewRepo[catalogProbeDoc](client, "", "services")}
		},
	})
}

type catalogProbeDoc struct {
	ID       bson.ObjectID `bson:"_id,omitempty"`
	ClinicID bson.ObjectID `bson:"clinicId,omitempty"`
	Marker   string        `bson:"marker"`
}

type catalogProbe struct {
	repo *tenantdb.Repo[catalogProbeDoc]
}

func (p *catalogProbe) InsertSample(ctx context.Context, marker string) error {
	return p.repo.Insert(ctx, &catalogProbeDoc{Marker: marker})
}

func (p *catalogProbe) CountVisible(ctx context.Context, filter any) (int64, error) {
	docs, err := p.repo.Find(ctx, filter)
	return int64(len(docs)), err
}

func (p *catalogProbe) UpdateSample(ctx context.Context, filter any) (int64, error) {
	res, err := p.repo.UpdateMany(ctx, filter, bson.M{"$set": bson.M{"marker": "tampered"}})
	if err != nil {
		return 0, err
	}
	return res.ModifiedCount, nil
}

func (p *catalogProbe) DeleteSample(ctx context.Context, filter any) (int64, error) {
	res, err := p.repo.DeleteMany(ctx, filter)
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

func (p *catalogProbe) AggregateCount(ctx context.Context) (int64, error) {
	res, err := p.repo.Aggregate(ctx, mongo.Pipeline{bson.D{{Key: "$count", Value: "total"}}})
	if err != nil || len(res) == 0 {
		return 0, err
	}
	switch v := res[0]["total"].(type) {
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	}
	return 0, nil
}
