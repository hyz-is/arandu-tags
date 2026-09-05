// What one entity carries, as a set.
//
// Attach and Detach in service.go answer about one association and report when
// the world was not what the caller thought it was. The calls here answer a
// different question: make this the set, and tell me what changed. A screen that
// submits the labels a form was rendered with is asking the second question, and
// asking the first once per label would refuse the whole submission because one
// of them was already there.

package tags

import (
	"context"
	"fmt"
	"slices"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
)

// AttachTags gives an entity every one of these labels, and reports how many it
// did not already carry.
//
// The labels it already carries are left alone rather than refused. That is what
// makes the call repeatable: the same request sent twice leaves the same set,
// and a form resubmitted by somebody's browser is not an error.
func (s *TagService) AttachTags(ctx context.Context, actor security.Subject, ref OwnerRef, tagIDs []string) (int, error) {
	g, wanted, err := s.authorizedForSet(ctx, actor, TagAttach, ref, tagIDs)
	if err != nil {
		return 0, err
	}

	carried, err := s.carriedIDs(ctx, g, ref)
	if err != nil {
		return 0, err
	}

	attached := 0
	for _, record := range wanted {
		if slices.Contains(carried, record.ID) {
			continue
		}
		if err := s.writeLink(ctx, g, ref, record.ID); err != nil {
			return attached, err
		}
		attached++
	}
	return attached, nil
}

// DetachTags takes every one of these labels back from an entity, and reports
// how many it was carrying.
//
// A label it does not carry is not an error here, for the reason one it already
// carries is not one in AttachTags: the call says what the set should be, and a
// label that is already absent is already what was asked for.
func (s *TagService) DetachTags(ctx context.Context, actor security.Subject, ref OwnerRef, tagIDs []string) (int, error) {
	g, wanted, err := s.authorizedForSet(ctx, actor, TagDetach, ref, tagIDs)
	if err != nil {
		return 0, err
	}

	removed, err := Taggables(s.db).NewQuery().
		Where("owner_type", "=", ref.Type().String()).
		Where("owner_id", "=", ref.ID()).
		WhereIn("tag_id", anys(orderedIDs(wanted))).
		Delete(ctx, g)
	if err != nil {
		return 0, err
	}
	return int(removed), nil
}

// DetachAllTags takes every label back from an entity, and reports how many
// there were.
//
// It is what an application calls when it deletes one of its own rows. This
// package cannot notice that deletion: the table is the application's, this
// package has never seen it, and there is no hook in Go that fires when somebody
// else's row goes away. Without this call the associations outlive the entity and
// answer questions about an identifier that has been reissued to something else.
func (s *TagService) DetachAllTags(ctx context.Context, actor security.Subject, ref OwnerRef) (int, error) {
	g, err := security.Authorize(ctx, s.policy, actor, TagDetach, Tag{})
	if err != nil {
		return 0, err
	}
	if ref.IsZero() {
		return 0, fmt.Errorf("%w: the reference names no entity", ErrOwnerType)
	}

	removed, err := Taggables(s.db).NewQuery().
		Where("owner_type", "=", ref.Type().String()).
		Where("owner_id", "=", ref.ID()).
		Delete(ctx, g)
	if err != nil {
		return 0, err
	}
	return int(removed), nil
}

// SyncTags makes these labels the whole of what an entity carries, across every
// taxonomy, and reports how many were attached and how many were removed.
//
// Everything not named is detached. That is the difference between this and
// AttachTags, and it is the reason the two are separate calls rather than one
// with a flag: a caller holding a complete set and a caller holding an addition
// are asking different questions, and a flag would let the wrong answer be one
// character away.
func (s *TagService) SyncTags(ctx context.Context, actor security.Subject, ref OwnerRef, tagIDs []string) (attached, detached int, err error) {
	return s.sync(ctx, actor, ref, anyTaxonomy, tagIDs)
}

// SyncTagsOfTaxonomy makes these labels the whole of what an entity carries
// within one taxonomy, leaving every other taxonomy alone.
//
// It is the call a screen makes, because a screen shows one taxonomy: the form
// that edits an article's statuses knows nothing about its languages, and a sync
// that detached them would delete data the person never saw.
func (s *TagService) SyncTagsOfTaxonomy(ctx context.Context, actor security.Subject, ref OwnerRef, taxonomy string, tagIDs []string) (attached, detached int, err error) {
	if !ValidTaxonomy(taxonomy) {
		return 0, 0, fmt.Errorf("tags: %q cannot be a taxonomy", taxonomy)
	}
	return s.sync(ctx, actor, ref, taxonomy, tagIDs)
}

// HasTag reports whether an entity carries this label.
func (s *TagService) HasTag(ctx context.Context, actor security.Subject, ref OwnerRef, tagID string) (bool, error) {
	g, err := security.Authorize(ctx, s.policy, actor, TagList, Tag{})
	if err != nil {
		return false, err
	}
	if ref.IsZero() {
		return false, fmt.Errorf("%w: the reference names no entity", ErrOwnerType)
	}
	if tagID == "" {
		return false, ErrNotFound
	}

	return Taggables(s.db).NewQuery().
		Where("tag_id", "=", tagID).
		Where("owner_type", "=", ref.Type().String()).
		Where("owner_id", "=", ref.ID()).
		Exists(ctx, g)
}

// TagsOfTaxonomy returns the labels of one taxonomy that an entity carries, in
// the order of that taxonomy.
//
// It filters in the statement rather than reading everything the entity carries
// and dropping what does not match here: an entity with labels in six
// taxonomies would otherwise pay for all six to be shown one.
func (s *TagService) TagsOfTaxonomy(ctx context.Context, actor security.Subject, ref OwnerRef, taxonomy string) ([]*Tag, error) {
	g, err := security.Authorize(ctx, s.policy, actor, TagList, Tag{})
	if err != nil {
		return nil, err
	}
	if ref.IsZero() {
		return nil, fmt.Errorf("%w: the reference names no entity", ErrOwnerType)
	}
	if !ValidTaxonomy(taxonomy) {
		return nil, fmt.Errorf("tags: %q cannot be a taxonomy", taxonomy)
	}

	carried, err := s.carriedIDs(ctx, g, ref)
	if err != nil || len(carried) == 0 {
		return nil, err
	}
	return Tags(s.db).NewQuery().
		Where("type", "=", taxonomy).
		WhereIn("id", anys(carried)).
		OrderBy("position").OrderBy("id").
		Get(ctx, g)
}

// OwnersWithAnyTagOfTaxonomy returns the entities of one kind that carry at
// least one label of any of these taxonomies.
//
// The question is about the taxonomies and not about particular labels: "every
// article that has a status", rather than "every article that is a draft". A
// caller asking it does not have to read the taxonomy first and pass its labels
// back in, which is the same question asked in two round trips and answered
// against a taxonomy that may have grown between them.
func (s *TagService) OwnersWithAnyTagOfTaxonomy(ctx context.Context, actor security.Subject, kind OwnerType, taxonomies []string) ([]string, error) {
	g, err := security.Authorize(ctx, s.policy, actor, TagList, Tag{})
	if err != nil {
		return nil, err
	}
	if kind.IsZero() {
		return nil, fmt.Errorf("%w: the kind is the zero value, which names nothing", ErrOwnerType)
	}
	switch {
	case len(taxonomies) == 0:
		return nil, fmt.Errorf("tags: no taxonomy was named, and a query about no taxonomy has no answer")
	case len(taxonomies) > MaxIDsPerQuery:
		return nil, fmt.Errorf("tags: %d taxonomies were named, and at most %d may be", len(taxonomies), MaxIDsPerQuery)
	}
	for _, taxonomy := range taxonomies {
		if !ValidTaxonomy(taxonomy) {
			return nil, fmt.Errorf("tags: %q cannot be a taxonomy", taxonomy)
		}
	}

	// Two statements over one table each, as everywhere else here: the labels of
	// those taxonomies, then the entities carrying them. A join would answer in
	// one round trip and would tie this package to a profile where one statement
	// may only name one table.
	labels, err := Tags(s.db).NewQuery().
		WhereIn("type", anys(distinct(taxonomies))).
		Limit(MaxIDsPerQuery).
		Pluck(ctx, g, "id")
	if err != nil {
		return nil, err
	}
	ids := texts(labels)
	if len(ids) == 0 {
		return nil, nil
	}
	return s.OwnersWithAnyTag(ctx, actor, kind, ids)
}

// UnusedTags returns the labels of one taxonomy that no entity carries, in the
// order of the taxonomy.
//
// It is what says which labels a taxonomy has accumulated and nothing uses. It
// does not say they are unwanted: a label created for next month is carried by
// nothing today, and deleting it is a decision this package does not take.
//
// It answers about a taxonomy that fits in one sweep, and refuses a larger one
// rather than answering about part of it. A partial answer here is a list of
// labels somebody is about to delete, and the ones it left out are the ones that
// are in use.
func (s *TagService) UnusedTags(ctx context.Context, actor security.Subject, taxonomy string) ([]*Tag, error) {
	g, err := security.Authorize(ctx, s.policy, actor, TagList, Tag{})
	if err != nil {
		return nil, err
	}
	if !ValidTaxonomy(taxonomy) {
		return nil, fmt.Errorf("tags: %q cannot be a taxonomy", taxonomy)
	}

	held, err := Tags(s.db).NewQuery().
		Where("type", "=", taxonomy).
		OrderBy("position").OrderBy("id").
		Limit(maxLimit+1).
		Get(ctx, g)
	if err != nil {
		return nil, err
	}
	if len(held) > maxLimit {
		return nil, fmt.Errorf("tags: %q holds more than %d labels, which is more than one sweep can answer about", taxonomy, maxLimit)
	}
	if len(held) == 0 {
		return nil, nil
	}

	carried, err := Taggables(s.db).NewQuery().
		WhereIn("tag_id", anys(orderedIDs(held))).
		Limit(maxLimit*MaxIDsPerQuery).
		Pluck(ctx, g, "tag_id")
	if err != nil {
		return nil, err
	}

	used := distinct(texts(carried))
	out := make([]*Tag, 0, len(held))
	for _, record := range held {
		if !slices.Contains(used, record.ID) {
			out = append(out, record)
		}
	}
	return out, nil
}

// anyTaxonomy is what sync is passed when the whole of what an entity carries is
// being replaced. It is not a taxonomy name and cannot collide with one:
// ValidTaxonomy refuses an upper-case letter.
const anyTaxonomy = "ANY"

// sync replaces what an entity carries, within one taxonomy or across all of
// them.
//
// The labels are loaded before anything is written, which is what proves they
// are this customer's, and the associations that go are read in the same shape
// they are compared in. Two writes, never one statement doing both: they are
// separate aggregates, and a transaction across them is the shape a module
// cannot keep on every profile it claims to support.
//
// What is removed goes first. A failure between the two leaves an entity
// carrying fewer labels than asked for, which the same call made again finishes;
// the other order would leave it carrying more, and a second call would remove
// the ones it had just been told to add.
func (s *TagService) sync(ctx context.Context, actor security.Subject, ref OwnerRef, taxonomy string, tagIDs []string) (attached, detached int, err error) {
	g, wanted, err := s.authorizedForSet(ctx, actor, TagAttach, ref, tagIDs)
	if err != nil {
		return 0, 0, err
	}
	if _, err := security.Authorize(ctx, s.policy, actor, TagDetach, Tag{}); err != nil {
		return 0, 0, err
	}
	for _, record := range wanted {
		if taxonomy != anyTaxonomy && record.Type != taxonomy {
			return 0, 0, fmt.Errorf("tags: %s is a label of %q and the sync is of %q", record.ID, record.Type, taxonomy)
		}
	}

	carried, err := s.carriedIDs(ctx, g, ref)
	if err != nil {
		return 0, 0, err
	}
	if taxonomy != anyTaxonomy && len(carried) > 0 {
		carried, err = s.narrowToTaxonomy(ctx, g, carried, taxonomy)
		if err != nil {
			return 0, 0, err
		}
	}

	keep := orderedIDs(wanted)
	var remove []string
	for _, id := range carried {
		if !slices.Contains(keep, id) {
			remove = append(remove, id)
		}
	}
	if len(remove) > 0 {
		removed, err := Taggables(s.db).NewQuery().
			Where("owner_type", "=", ref.Type().String()).
			Where("owner_id", "=", ref.ID()).
			WhereIn("tag_id", anys(remove)).
			Delete(ctx, g)
		if err != nil {
			return 0, 0, err
		}
		detached = int(removed)
	}

	for _, id := range keep {
		if slices.Contains(carried, id) {
			continue
		}
		if err := s.writeLink(ctx, g, ref, id); err != nil {
			return attached, detached, err
		}
		attached++
	}
	return attached, detached, nil
}

// authorizedForSet is the opening every set operation shares: decide, then load
// the labels named and decide about each of them.
//
// Loading them is not a convenience. An association naming a label of another
// customer would be a row that reads correctly and answers questions about
// somebody else's taxonomy, and the read is what makes that impossible -- the
// statement is scoped by tenant, so a label that is not this customer's is not
// found and the whole call is refused rather than partly written.
func (s *TagService) authorizedForSet(ctx context.Context, actor security.Subject, action security.Action, ref OwnerRef, tagIDs []string) (security.Grant, []*Tag, error) {
	g, err := security.Authorize(ctx, s.policy, actor, action, Tag{})
	if err != nil {
		return security.Grant{}, nil, err
	}
	if ref.IsZero() {
		return security.Grant{}, nil, fmt.Errorf("%w: the reference names no entity", ErrOwnerType)
	}
	if len(tagIDs) > MaxIDsPerQuery {
		return security.Grant{}, nil, fmt.Errorf("tags: %d tags were named, and at most %d may be", len(tagIDs), MaxIDsPerQuery)
	}
	wanted := distinct(tagIDs)
	if len(wanted) == 0 {
		return g, nil, nil
	}

	found, err := Tags(s.db).NewQuery().
		WhereIn("id", anys(wanted)).
		OrderBy("type").OrderBy("position").OrderBy("id").
		Get(ctx, g)
	if err != nil {
		return security.Grant{}, nil, err
	}
	if len(found) != len(wanted) {
		// Which one is missing is deliberately not said. A caller that could ask
		// for a hundred identifiers and be told which of them exist has a way to
		// enumerate another customer's taxonomy one refusal at a time.
		return security.Grant{}, nil, fmt.Errorf("%w: %d of %d labels", ErrNotFound, len(wanted)-len(found), len(wanted))
	}
	for _, record := range found {
		if _, err := security.Authorize(ctx, s.policy, actor, action, *record); err != nil {
			return security.Grant{}, nil, err
		}
	}
	return g, found, nil
}

// carriedIDs are the labels one entity carries, as identifiers.
func (s *TagService) carriedIDs(ctx context.Context, g security.Grant, ref OwnerRef) ([]string, error) {
	found, err := Taggables(s.db).NewQuery().
		Where("owner_type", "=", ref.Type().String()).
		Where("owner_id", "=", ref.ID()).
		Limit(maxLimit).
		Pluck(ctx, g, "tag_id")
	if err != nil {
		return nil, err
	}
	return distinct(texts(found)), nil
}

// narrowToTaxonomy keeps only the labels of one taxonomy, by asking the labels
// rather than by reading their taxonomy off the association -- which does not
// carry one, and would be a second place for it to be written down.
func (s *TagService) narrowToTaxonomy(ctx context.Context, g security.Grant, ids []string, taxonomy string) ([]string, error) {
	found, err := Tags(s.db).NewQuery().
		Where("type", "=", taxonomy).
		WhereIn("id", anys(ids)).
		Limit(maxLimit).
		Pluck(ctx, g, "id")
	if err != nil {
		return nil, err
	}
	return distinct(texts(found)), nil
}

// writeLink writes one association.
func (s *TagService) writeLink(ctx context.Context, g security.Grant, ref OwnerRef, tagID string) error {
	id, err := data.NewID()
	if err != nil {
		return err
	}
	instance, err := Taggables(s.db).NewInstance(nil, false)
	if err != nil {
		return err
	}
	link := instance.Entity
	link.ID = id
	link.TenantID = data.Tenant(g)
	link.TagID = tagID
	link.OwnerType = ref.Type().String()
	link.OwnerID = ref.ID()
	_, err = link.Save(ctx, g)
	return err
}
