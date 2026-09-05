package tags

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/framework/validation"
	"github.com/arandu-io/hesape/database/model"
)

// Pagination bounds for List. A request that asks for everything gets the
// maximum, never everything: an unbounded query is how one page load takes a
// production database down.
const (
	defaultLimit = 50
	maxLimit     = 200

	// MaxNameLen is the longest a label may be.
	MaxNameLen = 120

	// MaxIDsPerQuery bounds how many identifiers one association query may
	// name. The list becomes an IN clause, and an IN clause with no ceiling is
	// a statement whose cost the caller sets.
	MaxIDsPerQuery = 200

	// PositionStep is the distance between two positions handed out in a row.
	//
	// Positions are sparse so that a label can be moved between two others by
	// naming a number between theirs, without renumbering everything after it.
	// A move that finds no room is refused rather than silently shifting rows
	// nobody asked about.
	PositionStep = 1024

	// claimAttempts bounds how many times a position claim is retried when
	// another writer took the value it had read. Each attempt loses only to a
	// claim that succeeded, so the loop ends as soon as this writer is the one
	// that wins.
	//
	// It is a safety valve and not a tuning knob: with the backoff below, the
	// wait before giving up is under a second, and a caller that is told to try
	// again is better off than one held indefinitely on a counter it keeps
	// losing.
	claimAttempts = 64

	// claimBackoff is how long a lost claim waits before reading again, and
	// claimBackoffCap is as long as that wait ever gets.
	//
	// Waiting is what makes the retry bounded in practice. Without it every
	// loser reads again immediately, so N writers on one counter take N turns
	// each and the writer that wins last has lost N-1 times. Spreading them out
	// in time leaves few enough contenders per round that each one wins within
	// a handful.
	claimBackoff    = 200 * time.Microsecond
	claimBackoffCap = 5 * time.Millisecond
)

// sortableTag is the ordering allowlist. A column name taken directly
// from a request would turn ordering into an injection surface.
//
// The default is the position, because a taxonomy that persists an order
// exists to be read in it.
var sortableTag = map[string]string{
	"":           "position",
	"position":   "position",
	"name":       "name",
	"created_at": "created_at",
}

// TagService holds the rules of this package.
//
// It receives its collaborators through the constructor. There is no container
// and no resolution by reflection: what this service is made of is written at
// the one place that builds it, and reading that place is how somebody learns
// what the package touches.
//
// Everything a handler is allowed to do goes through here. The service is the
// only owner of the database handle, so the request layer cannot reach a Model
// before the policy has answered.
//
// One service owns all three tables. The labels, the associations and the
// counter that hands out positions are one boundary: an association is only
// meaningful about a label this tenant holds, and splitting them would put the
// check that says so on the caller's side of the line.
//
// The policy is held as the contract rather than as the concrete type, and the
// field is unexported, so the constructor below is the only place that says
// which policy decides. There is no setter and no option: an application that
// wants different rules edits TagPolicy, which is the one place rules live.
type TagService struct {
	db     *data.DB
	policy security.Policy[Tag]
}

// NewTagService wires the service over the application's database handle.
func NewTagService(db *data.DB) *TagService {
	return &TagService{db: db, policy: TagPolicy{}}
}

// CreateRequest is the input contract.
//
// The fields are explicit and there is no mass assignment, so a request body
// cannot write a column nobody meant to expose. There is no TenantID here and
// there must never be one: the tenant comes from the Grant, which comes from
// the session.
//
// There is no Position either, and for the same kind of reason: a position is
// claimed, and a caller that named one would be naming a place it has not been
// told is free.
type CreateRequest struct {
	// Name is what the label will be called.
	Name string
	// Taxonomy is the taxonomy the label joins. Empty is DefaultTaxonomy.
	Taxonomy string
}

// Validate reports the errors per field.
func (r CreateRequest) Validate() validation.Errors {
	e := validation.Errors{}
	validation.Required(e, "name", r.Name)
	validation.MaxLen(e, "name", r.Name, MaxNameLen)
	// A name of punctuation alone reduces to no slug, and a label with no slug
	// cannot be addressed or told apart from the next one.
	if r.Name != "" && Slugify(r.Name) == "" {
		e.Add("name", "must hold at least one letter or digit")
	}
	if !ValidTaxonomy(r.Taxonomy) {
		e.Add("type", "may hold lowercase letters, digits, - and _, and has to start with a letter")
	}
	return e
}

// Compile-time proof that the request honors the validation contract.
var _ validation.Validatable = CreateRequest{}

// RenameRequest changes what a label is called.
//
// It does not change the slug. The slug is what other rows and other systems
// have written down, and rewriting it on a rename would move the label out
// from under everything that addressed it.
type RenameRequest struct {
	// Name is what the label will be called from now on.
	Name string
}

// Validate reports the errors per field.
func (r RenameRequest) Validate() validation.Errors {
	e := validation.Errors{}
	validation.Required(e, "name", r.Name)
	validation.MaxLen(e, "name", r.Name, MaxNameLen)
	return e
}

// Compile-time proof that the request honors the validation contract.
var _ validation.Validatable = RenameRequest{}

// Create adds a label to a taxonomy: validate, authorize, then act with the
// Grant the authorization produced.
//
// The candidate is authorized before it is stored, and the candidate is what
// the policy sees -- so a rule about what may be created is a rule about the
// record being created, and not about the person alone.
//
// The slug is checked for a taxonomy that already holds it, and the check is a
// courtesy rather than the guarantee: two callers can both find it free. What
// makes a duplicate impossible is the unique index over tenant, taxonomy and
// slug, and the loser of that race gets the engine's refusal rather than
// ErrSlugTaken. Neither of them writes a second row.
func (s *TagService) Create(ctx context.Context, actor security.Subject, in CreateRequest) (*Tag, error) {
	if errs := in.Validate(); errs.Any() {
		return nil, errs
	}

	proposed := Tag{Type: in.Taxonomy, Name: in.Name, Slug: Slugify(in.Name)}

	g, err := security.Authorize(ctx, s.policy, actor, TagCreate, proposed)
	if err != nil {
		return nil, err
	}

	taken, err := Tags(s.db).NewQuery().
		Where("type", "=", proposed.Type).
		Where("slug", "=", proposed.Slug).
		Exists(ctx, g)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, fmt.Errorf("%w: %q in %q", ErrSlugTaken, proposed.Slug, proposed.Type)
	}

	position, err := s.claimPosition(ctx, g, proposed.Type)
	if err != nil {
		return nil, err
	}

	id, err := data.NewID()
	if err != nil {
		return nil, err
	}
	instance, err := Tags(s.db).NewInstance(nil, false)
	if err != nil {
		return nil, err
	}
	candidate := instance.Entity
	candidate.ID = id
	candidate.TenantID = data.Tenant(g)
	candidate.Type = proposed.Type
	candidate.Name = proposed.Name
	candidate.Slug = proposed.Slug
	candidate.Position = position
	if _, err := candidate.Save(ctx, g); err != nil {
		return nil, err
	}
	return candidate, nil
}

// Find returns one record, and asks the policy twice.
//
// The first call is on the empty candidate, because there is no way to read the
// record without a Grant and no way to hold a Grant without a decision. What it
// decides is whether this subject may view records of this kind at all.
//
// The second call is on the record that came back, and it is the one a rule
// about the record itself depends on: the first call saw an empty value, so
// anything the policy says about who owns the row, or about a row that is not
// published yet, never ran. Without it a policy can be written that looks
// correct, reads correctly, and is never consulted about the thing it protects.
//
// The read itself is already scoped by data.Tenant, so the second call is not
// what keeps customers apart. It is what keeps the policy honest.
func (s *TagService) Find(ctx context.Context, actor security.Subject, id string) (*Tag, error) {
	g, err := security.Authorize(ctx, s.policy, actor, TagView, Tag{})
	if err != nil {
		return nil, err
	}

	record, err := Tags(s.db).NewQuery().WhereKey(id).First(ctx, g)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, ErrNotFound
	}

	if _, err := security.Authorize(ctx, s.policy, actor, TagView, *record); err != nil {
		return nil, err
	}
	return record, nil
}

// List returns a page of one taxonomy.
//
// The taxonomy is a parameter rather than a filter that may be left out,
// because a taxonomy is the unit a listing is read in: a page mixing the
// statuses of a document with the languages of an article is a page nobody
// asked for. DefaultTaxonomy is a taxonomy like any other and is listed the
// same way.
//
// It authorizes once, on the empty candidate, and the tenant filter in the
// statement is what bounds the rows. A policy call per row would be one call
// per record on a page and would still not narrow the query -- a listing that
// has to read a customer's rows in order to decide it may not read them has
// already read them.
//
// A rule that hides individual records from a listing belongs in the statement,
// as a predicate, and the action here is what decides whether the listing may
// run at all.
func (s *TagService) List(ctx context.Context, actor security.Subject, taxonomy string, q data.Query) ([]*Tag, error) {
	g, err := security.Authorize(ctx, s.policy, actor, TagList, Tag{})
	if err != nil {
		return nil, err
	}
	if !ValidTaxonomy(taxonomy) {
		return nil, fmt.Errorf("tags: %q cannot be a taxonomy", taxonomy)
	}

	column, ok := sortableTag[q.Sort]
	if !ok {
		return nil, fmt.Errorf("tags: sort field not allowed: %q", q.Sort)
	}

	limit := q.Limit
	switch {
	case limit <= 0:
		limit = defaultLimit
	case limit > maxLimit:
		limit = maxLimit
	}

	rows := Tags(s.db)
	page := rows.NewQuery().Where("type", "=", taxonomy)
	if q.Cursor != "" {
		anchor, err := rows.NewQuery().WhereKey(q.Cursor).Value(ctx, g, column)
		if err != nil {
			return nil, err
		}
		if anchor == nil {
			return nil, nil
		}
		page = page.Where(func(after *model.Builder[Tag]) {
			after.Where(column, ">", anchor).
				OrWhere(func(equal *model.Builder[Tag]) {
					equal.Where(column, "=", anchor).Where("id", ">", q.Cursor)
				})
		})
	}

	return page.OrderBy(column).OrderBy("id").Limit(limit).Get(ctx, g)
}

// Rename changes what a label is called, leaving its slug and its place alone.
func (s *TagService) Rename(ctx context.Context, actor security.Subject, id string, in RenameRequest) (*Tag, error) {
	if errs := in.Validate(); errs.Any() {
		return nil, errs
	}

	g, err := security.Authorize(ctx, s.policy, actor, TagUpdate, Tag{})
	if err != nil {
		return nil, err
	}

	record, err := Tags(s.db).NewQuery().WhereKey(id).First(ctx, g)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, ErrNotFound
	}
	if _, err := security.Authorize(ctx, s.policy, actor, TagUpdate, *record); err != nil {
		return nil, err
	}

	record.Name = in.Name
	if _, err := record.Save(ctx, g); err != nil {
		return nil, err
	}
	return record, nil
}

// Move puts a label at a position of the caller's choosing within its
// taxonomy.
//
// Positions are handed out PositionStep apart, so the room between two
// neighbours is where a label goes when it is moved between them. A position
// another label already holds is refused by the unique index rather than taken,
// because the alternative is renumbering rows the caller said nothing about.
func (s *TagService) Move(ctx context.Context, actor security.Subject, id string, position int64) (*Tag, error) {
	g, err := security.Authorize(ctx, s.policy, actor, TagUpdate, Tag{})
	if err != nil {
		return nil, err
	}
	if position < 0 {
		return nil, fmt.Errorf("tags: the position is %d, and cannot be negative", position)
	}

	record, err := Tags(s.db).NewQuery().WhereKey(id).First(ctx, g)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, ErrNotFound
	}
	if _, err := security.Authorize(ctx, s.policy, actor, TagUpdate, *record); err != nil {
		return nil, err
	}

	record.Position = position
	if _, err := record.Save(ctx, g); err != nil {
		return nil, err
	}
	return record, nil
}

// Delete removes a label and every association to it.
//
// The associations go first. There is no transaction across the two tables --
// they are separate aggregates, and a write that spans them is the one shape a
// module cannot keep on every profile it claims to support -- so the order is
// what decides what a failure between them leaves behind. Associations first
// leaves a label nothing points at, which a second call finishes. The other
// order would leave associations naming a label that is gone, which nothing
// reports and every reader has to defend against.
func (s *TagService) Delete(ctx context.Context, actor security.Subject, id string) error {
	g, err := security.Authorize(ctx, s.policy, actor, TagDelete, Tag{})
	if err != nil {
		return err
	}

	record, err := Tags(s.db).NewQuery().WhereKey(id).First(ctx, g)
	if err != nil {
		return err
	}
	if record == nil {
		return ErrNotFound
	}
	if _, err := security.Authorize(ctx, s.policy, actor, TagDelete, *record); err != nil {
		return err
	}

	if _, err := Taggables(s.db).NewQuery().Where("tag_id", "=", record.ID).Delete(ctx, g); err != nil {
		return err
	}
	if _, err := record.Delete(ctx, g); err != nil {
		return err
	}
	return nil
}

// Attach gives a label to an entity.
//
// The label is loaded before the association is written, which is what proves
// the label is this tenant's: an association naming a label of another customer
// would be a row that reads correctly and answers questions about somebody
// else's taxonomy.
//
// An entity that already carries the label is reported rather than written
// twice, and the unique index over tenant, label, kind and identifier is what
// makes that true when two callers ask at once.
func (s *TagService) Attach(ctx context.Context, actor security.Subject, ref OwnerRef, tagID string) error {
	g, err := security.Authorize(ctx, s.policy, actor, TagAttach, Tag{})
	if err != nil {
		return err
	}
	if ref.IsZero() {
		return fmt.Errorf("%w: the reference names no entity", ErrOwnerType)
	}

	record, err := Tags(s.db).NewQuery().WhereKey(tagID).First(ctx, g)
	if err != nil {
		return err
	}
	if record == nil {
		return ErrNotFound
	}
	if _, err := security.Authorize(ctx, s.policy, actor, TagAttach, *record); err != nil {
		return err
	}

	carried, err := Taggables(s.db).NewQuery().
		Where("tag_id", "=", record.ID).
		Where("owner_type", "=", ref.Type().String()).
		Where("owner_id", "=", ref.ID()).
		Exists(ctx, g)
	if err != nil {
		return err
	}
	if carried {
		return fmt.Errorf("%w: %s carries %s", ErrAlreadyAttached, ref, record.ID)
	}
	return s.writeLink(ctx, g, ref, record.ID)
}

// Detach takes a label back from an entity.
//
// An entity that does not carry it is reported rather than passed over: a
// caller that asked to remove something and was told nothing happened has
// learned that its idea of the world is wrong, which is the point of asking.
func (s *TagService) Detach(ctx context.Context, actor security.Subject, ref OwnerRef, tagID string) error {
	g, err := security.Authorize(ctx, s.policy, actor, TagDetach, Tag{})
	if err != nil {
		return err
	}
	if ref.IsZero() {
		return fmt.Errorf("%w: the reference names no entity", ErrOwnerType)
	}

	record, err := Tags(s.db).NewQuery().WhereKey(tagID).First(ctx, g)
	if err != nil {
		return err
	}
	if record == nil {
		return ErrNotFound
	}
	if _, err := security.Authorize(ctx, s.policy, actor, TagDetach, *record); err != nil {
		return err
	}

	removed, err := Taggables(s.db).NewQuery().
		Where("tag_id", "=", record.ID).
		Where("owner_type", "=", ref.Type().String()).
		Where("owner_id", "=", ref.ID()).
		Delete(ctx, g)
	if err != nil {
		return err
	}
	if removed == 0 {
		return fmt.Errorf("%w: %s does not carry %s", ErrNotAttached, ref, record.ID)
	}
	return nil
}

// TagsOf returns the labels one entity carries, in the order of their
// taxonomies.
//
// Two statements, each over one table. The associations name the labels and
// the labels are then read by identifier: a join would answer in one round
// trip and would tie this package to a profile where one statement may only
// name one table.
func (s *TagService) TagsOf(ctx context.Context, actor security.Subject, ref OwnerRef) ([]*Tag, error) {
	g, err := security.Authorize(ctx, s.policy, actor, TagList, Tag{})
	if err != nil {
		return nil, err
	}
	if ref.IsZero() {
		return nil, fmt.Errorf("%w: the reference names no entity", ErrOwnerType)
	}

	carried, err := Taggables(s.db).NewQuery().
		Where("owner_type", "=", ref.Type().String()).
		Where("owner_id", "=", ref.ID()).
		Limit(maxLimit).
		Pluck(ctx, g, "tag_id")
	if err != nil {
		return nil, err
	}
	ids := texts(carried)
	if len(ids) == 0 {
		return nil, nil
	}

	return Tags(s.db).NewQuery().
		WhereIn("id", anys(ids)).
		OrderBy("type").OrderBy("position").OrderBy("id").
		Get(ctx, g)
}

// OwnersWithAnyTag returns the entities of one kind that carry at least one of
// these labels.
//
// It answers with identifiers rather than with entities, and that is the whole
// shape of this package's side of the association: the table on the other side
// belongs to the application, this package has never seen it, and a module
// that returned rows out of it would be a module that had been told how to
// read somebody else's schema.
func (s *TagService) OwnersWithAnyTag(ctx context.Context, actor security.Subject, kind OwnerType, tagIDs []string) ([]string, error) {
	g, err := security.Authorize(ctx, s.policy, actor, TagList, Tag{})
	if err != nil {
		return nil, err
	}
	if err := checkOwnerQuery(kind, tagIDs); err != nil {
		return nil, err
	}

	found, err := Taggables(s.db).NewQuery().
		Where("owner_type", "=", kind.String()).
		WhereIn("tag_id", anys(tagIDs)).
		Pluck(ctx, g, "owner_id")
	if err != nil {
		return nil, err
	}
	return distinct(texts(found)), nil
}

// OwnersWithAllTags returns the entities of one kind that carry every one of
// these labels.
//
// The counting is done by the engine rather than by reading every association
// back and grouping them here: the answer is the entities, and the rows that
// prove it are not something a caller asked to pay for.
func (s *TagService) OwnersWithAllTags(ctx context.Context, actor security.Subject, kind OwnerType, tagIDs []string) ([]string, error) {
	g, err := security.Authorize(ctx, s.policy, actor, TagList, Tag{})
	if err != nil {
		return nil, err
	}
	if err := checkOwnerQuery(kind, tagIDs); err != nil {
		return nil, err
	}

	wanted := distinct(tagIDs)
	grouped := Taggables(s.db).NewQuery().
		Where("owner_type", "=", kind.String()).
		WhereIn("tag_id", anys(wanted)).
		GroupBy("owner_id")
	// The label list is deduplicated above, so counting distinct labels per
	// entity and comparing with its length is the same question as "carries
	// all of them" -- and it stays that way when the same label is named twice.
	grouped.GetQuery().HavingRaw("count(distinct tag_id) = ?", len(wanted))

	found, err := grouped.Pluck(ctx, g, "owner_id")
	if err != nil {
		return nil, err
	}
	return distinct(texts(found)), nil
}

// OwnersWithoutAnyTag returns, of the entities named in candidates, the ones
// that carry none of these labels.
//
// The candidates are a parameter and not something this package works out for
// itself, and that is not a shortcut. This package holds associations, not
// entities: it knows an article exists only once somebody has tagged it, so
// "every article with none of these labels" is a question about a table it has
// never read. A caller has that table, pages through it, and asks about the
// page it is holding -- which is also the only form of the question with a
// bounded answer.
func (s *TagService) OwnersWithoutAnyTag(ctx context.Context, actor security.Subject, kind OwnerType, tagIDs []string, candidates []string) ([]string, error) {
	g, err := security.Authorize(ctx, s.policy, actor, TagList, Tag{})
	if err != nil {
		return nil, err
	}
	if err := checkOwnerQuery(kind, tagIDs); err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	if len(candidates) > MaxIDsPerQuery {
		return nil, fmt.Errorf("tags: %d candidates were named, and at most %d may be", len(candidates), MaxIDsPerQuery)
	}

	remaining := distinct(candidates)
	found, err := Taggables(s.db).NewQuery().
		Where("owner_type", "=", kind.String()).
		WhereIn("tag_id", anys(tagIDs)).
		WhereIn("owner_id", anys(remaining)).
		Pluck(ctx, g, "owner_id")
	if err != nil {
		return nil, err
	}

	carrying := distinct(texts(found))
	out := make([]string, 0, len(remaining))
	for _, candidate := range remaining {
		if !slices.Contains(carrying, candidate) {
			out = append(out, candidate)
		}
	}
	return out, nil
}

// claimPosition reserves the next position in one taxonomy.
func (s *TagService) claimPosition(ctx context.Context, g security.Grant, taxonomy string) (int64, error) {
	return s.claimPositions(ctx, g, taxonomy, 1)
}

// claimPositions reserves count consecutive positions in one taxonomy and
// returns the first of them. They are PositionStep apart, so the caller writes
// first, first+PositionStep, and so on.
//
// The value written is conditional on the value read. The update matches the
// counter only while it still holds what the read returned, so a claim that
// raced another one changes no row, learns that it lost, and reads again. What
// this replaces is reading the largest position in use and writing one past it:
// nothing there connects the read to the write, so two writers read the same
// number, both write it, and the order those rows were meant to establish has
// two of them in one place.
//
// A whole block is claimed in one turn of that loop rather than one position at
// a time, so a caller placing many labels at once takes the counter once and the
// positions it gets cannot be interleaved with another writer's.
//
// The counter row is created on the first claim of a taxonomy. Two callers can
// both find it missing and both try to create it, and the loser is recognised
// by asking the table rather than by reading the driver's error: a row that is
// there now says the race happened and the claim goes on, and a row that is
// still missing says the write failed for a reason this cannot fix.
func (s *TagService) claimPositions(ctx context.Context, g security.Grant, taxonomy string, count int) (int64, error) {
	if count < 1 {
		return 0, fmt.Errorf("tags: %d positions were claimed, and at least one has to be", count)
	}
	key := sequenceKey(data.Tenant(g), taxonomy)
	counters := tagSequences(s.db)
	width := int64(count) * PositionStep

	for attempt := 0; attempt < claimAttempts; attempt++ {
		counter, err := counters.NewQuery().WhereKey(key).First(ctx, g)
		if err != nil {
			return 0, err
		}
		if counter == nil {
			counter, err = s.seedSequence(ctx, g, key, taxonomy)
			if err != nil {
				return 0, err
			}
		}

		claimed := counter.NextPosition
		changed, err := counters.NewQuery().
			WhereKey(key).
			Where("next_position", "=", claimed).
			Update(ctx, g, map[string]any{"next_position": claimed + width})
		if err != nil {
			return 0, err
		}
		if changed == 1 {
			return claimed, nil
		}
		if err := waitToClaimAgain(ctx, attempt); err != nil {
			return 0, err
		}
	}
	return 0, fmt.Errorf("%w: %d attempts on %q", ErrPositionUnavailable, claimAttempts, taxonomy)
}

// waitToClaimAgain pauses a claim that lost, for longer each time it loses.
//
// The pause is randomised across the interval rather than taken from it whole:
// writers that lost the same round would otherwise wake together and contend
// again as one crowd, which is the round they just lost repeated.
//
// It watches the context, so a request that was abandoned stops here instead of
// sleeping out its remaining attempts.
func waitToClaimAgain(ctx context.Context, attempt int) error {
	window := claimBackoff << min(attempt, 8)
	if window > claimBackoffCap {
		window = claimBackoffCap
	}
	timer := time.NewTimer(time.Duration(rand.Int64N(int64(window)) + 1))
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// seedSequence writes the counter row of a taxonomy that has none yet, and
// returns whichever row ends up being there.
func (s *TagService) seedSequence(ctx context.Context, g security.Grant, key, taxonomy string) (*tagSequence, error) {
	instance, err := tagSequences(s.db).NewInstance(nil, false)
	if err != nil {
		return nil, err
	}
	counter := instance.Entity
	counter.ID = key
	counter.TenantID = data.Tenant(g)
	counter.Type = taxonomy
	counter.NextPosition = 0

	if _, err := counter.Save(ctx, g); err == nil {
		return counter, nil
	} else {
		existing, readErr := tagSequences(s.db).NewQuery().WhereKey(key).First(ctx, g)
		if readErr != nil {
			return nil, readErr
		}
		if existing == nil {
			return nil, err
		}
		return existing, nil
	}
}

// checkOwnerQuery refuses an association query that cannot be answered, before
// it becomes a statement.
func checkOwnerQuery(kind OwnerType, tagIDs []string) error {
	if kind.IsZero() {
		return fmt.Errorf("%w: the kind is the zero value, which names nothing", ErrOwnerType)
	}
	if len(tagIDs) == 0 {
		return fmt.Errorf("tags: no tag was named, and a query about no tag has no answer")
	}
	if len(tagIDs) > MaxIDsPerQuery {
		return fmt.Errorf("tags: %d tags were named, and at most %d may be", len(tagIDs), MaxIDsPerQuery)
	}
	return nil
}

// texts turns the values a pluck returned into strings, skipping anything that
// is not one. A column declared as text answers as text, and a value that
// arrives as something else is a row this package did not write.
func texts(values []any) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		switch v := value.(type) {
		case string:
			out = append(out, v)
		case []byte:
			out = append(out, string(v))
		}
	}
	return out
}

// anys widens identifiers for the bindings of an IN clause.
func anys(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

// distinct returns the values once each, in ascending order, so that one
// answer to one question reads the same every time it is asked.
func distinct(values []string) []string {
	out := slices.Clone(values)
	slices.Sort(out)
	return slices.Compact(out)
}
