// Where a label sits in its taxonomy, and every way it is moved.
//
// One mechanism underneath all of them: positions are claimed from the counter,
// PositionStep apart, and a row is written to a new one only while it still
// holds the one that was read. Nothing here renumbers a taxonomy to make room,
// and nothing here reads the largest position in use and writes one past it --
// that is the read that is not connected to its write, and it puts two labels in
// one place the first time two writers overlap.

package tags

import (
	"context"
	"errors"
	"fmt"

	"github.com/arandu-io/framework/security"
)

// The refusals ordering answers with.
var (
	// ErrOrderIncomplete is returned when a reorder names a set of labels that
	// is not the taxonomy's own.
	ErrOrderIncomplete = errors.New("tags: the order does not name exactly the labels of the taxonomy")

	// ErrOrderChanged is returned when another writer moved one of the labels
	// while this call was rewriting the order. It is a lost race and not a
	// refusal: the same call made again is expected to succeed.
	ErrOrderChanged = errors.New("tags: the order changed while it was being rewritten")

	// ErrDifferentTaxonomies is returned when two labels that are not in one
	// taxonomy are asked to exchange places. Positions are only comparable
	// within a taxonomy, so the exchange would name no order at all.
	ErrDifferentTaxonomies = errors.New("tags: the labels are in different taxonomies")
)

// Reorder writes one order over a whole taxonomy, in the order the identifiers
// are given.
//
// It takes every label of the taxonomy and refuses a partial list. A reorder of
// some of them leaves the rest where they were, which produces an order the
// caller did not describe and cannot predict -- the unnamed labels keep old
// positions and interleave with the new ones wherever those happen to land.
//
// The new positions are one block claimed from the counter, so they are past
// everything in use and no row passes through a position another row holds. The
// block is claimed once rather than per label, which is what keeps the labels
// contiguous when another writer is creating labels at the same time.
func (s *TagService) Reorder(ctx context.Context, actor security.Subject, taxonomy string, ids []string) ([]*Tag, error) {
	g, err := security.Authorize(ctx, s.policy, actor, TagUpdate, Tag{})
	if err != nil {
		return nil, err
	}
	if !ValidTaxonomy(taxonomy) {
		return nil, fmt.Errorf("tags: %q cannot be a taxonomy", taxonomy)
	}
	switch {
	case len(ids) == 0:
		return nil, fmt.Errorf("%w: no label was named", ErrOrderIncomplete)
	case len(ids) > MaxIDsPerQuery:
		return nil, fmt.Errorf("tags: %d labels were named, and at most %d may be", len(ids), MaxIDsPerQuery)
	case len(distinct(ids)) != len(ids):
		return nil, fmt.Errorf("%w: a label is named twice, and one label has one place", ErrOrderIncomplete)
	}

	// One more than the ceiling, so a taxonomy too large to order in one call is
	// told so rather than silently reordered against the first MaxIDsPerQuery of
	// it.
	held, err := Tags(s.db).NewQuery().
		Where("type", "=", taxonomy).
		OrderBy("position").OrderBy("id").
		Limit(MaxIDsPerQuery+1).
		Get(ctx, g)
	if err != nil {
		return nil, err
	}
	if len(held) > MaxIDsPerQuery {
		return nil, fmt.Errorf("tags: %q holds more than %d labels, which is more than one order can name", taxonomy, MaxIDsPerQuery)
	}

	byID := make(map[string]*Tag, len(held))
	for _, record := range held {
		byID[record.ID] = record
	}
	if len(held) != len(ids) {
		return nil, fmt.Errorf("%w: %d were named and %q holds %d", ErrOrderIncomplete, len(ids), taxonomy, len(held))
	}
	ordered := make([]*Tag, 0, len(ids))
	for _, id := range ids {
		record, held := byID[id]
		if !held {
			return nil, fmt.Errorf("%w: %s is not a label of %q", ErrOrderIncomplete, id, taxonomy)
		}
		// Every row here is written, so every row is decided about. A reorder
		// that authorized once on the empty candidate would be a write on N
		// records the policy was never asked about.
		if _, err := security.Authorize(ctx, s.policy, actor, TagUpdate, *record); err != nil {
			return nil, err
		}
		ordered = append(ordered, record)
	}

	base, err := s.claimPositions(ctx, g, taxonomy, len(ordered))
	if err != nil {
		return nil, err
	}
	for i, record := range ordered {
		position := base + int64(i)*PositionStep
		if err := s.writePosition(ctx, g, record, position); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

// MoveUp exchanges a label with the one before it in its taxonomy.
//
// A label that is already first is answered unchanged and nothing is written: it
// is where the caller asked for it to be, and reporting that as a failure would
// make a screen's up arrow an error at the top of every list.
func (s *TagService) MoveUp(ctx context.Context, actor security.Subject, id string) (*Tag, error) {
	return s.moveBeside(ctx, actor, id, "<", "desc")
}

// MoveDown exchanges a label with the one after it in its taxonomy.
//
// A label that is already last is answered unchanged, for the reason a first one
// is on the way up.
func (s *TagService) MoveDown(ctx context.Context, actor security.Subject, id string) (*Tag, error) {
	return s.moveBeside(ctx, actor, id, ">", "asc")
}

// MoveToStart puts a label before every other label of its taxonomy.
//
// The position it takes is PositionStep below the lowest one in use, which is
// below zero once the taxonomy has never been reordered. That is the shape of
// the column: the position is an ordering key over signed integers, and zero is
// where the first claim happens to land rather than a floor. Keeping zero as a
// floor would mean renumbering the taxonomy to open room at the front, which is
// the work sparse positions exist to avoid.
func (s *TagService) MoveToStart(ctx context.Context, actor security.Subject, id string) (*Tag, error) {
	g, record, err := s.authorizedForMove(ctx, actor, id)
	if err != nil {
		return nil, err
	}

	edge, err := s.edgePosition(ctx, g, record.Type, "asc")
	if err != nil {
		return nil, err
	}
	if edge == record.Position {
		return record, nil
	}
	if err := s.writePosition(ctx, g, record, edge-PositionStep); err != nil {
		return nil, err
	}
	return record, nil
}

// MoveToEnd puts a label after every other label of its taxonomy.
//
// The position is claimed from the counter, which is what already hands out a
// place past everything in use, so a label sent to the end takes the place the
// next created label would have taken.
func (s *TagService) MoveToEnd(ctx context.Context, actor security.Subject, id string) (*Tag, error) {
	g, record, err := s.authorizedForMove(ctx, actor, id)
	if err != nil {
		return nil, err
	}

	edge, err := s.edgePosition(ctx, g, record.Type, "desc")
	if err != nil {
		return nil, err
	}
	if edge == record.Position {
		return record, nil
	}
	position, err := s.claimPosition(ctx, g, record.Type)
	if err != nil {
		return nil, err
	}
	if err := s.writePosition(ctx, g, record, position); err != nil {
		return nil, err
	}
	return record, nil
}

// SwapOrder exchanges the places of two labels of one taxonomy.
//
// Both are decided about, because both are written. Labels of two taxonomies are
// refused: a position means something within a taxonomy and nothing across two,
// so the exchange would describe no order and could land on a position the other
// taxonomy already holds.
func (s *TagService) SwapOrder(ctx context.Context, actor security.Subject, firstID, secondID string) error {
	g, first, err := s.authorizedForMove(ctx, actor, firstID)
	if err != nil {
		return err
	}
	if firstID == secondID {
		// One label exchanged with itself is the order it already has, and
		// writing it would be two conditional updates that undo each other.
		return nil
	}

	second, err := Tags(s.db).NewQuery().WhereKey(secondID).First(ctx, g)
	if err != nil {
		return err
	}
	if second == nil {
		return ErrNotFound
	}
	if _, err := security.Authorize(ctx, s.policy, actor, TagUpdate, *second); err != nil {
		return err
	}
	if first.Type != second.Type {
		return fmt.Errorf("%w: %q and %q", ErrDifferentTaxonomies, first.Type, second.Type)
	}
	return s.swapPositions(ctx, g, first, second)
}

// moveBeside exchanges a label with its neighbour on one side.
//
// The neighbour is read by the comparison and the direction the caller names:
// the nearest row on that side of this one, within the taxonomy. Nothing is
// written when there is none, because a label at either edge is already as far
// as it goes.
func (s *TagService) moveBeside(ctx context.Context, actor security.Subject, id, comparison, direction string) (*Tag, error) {
	g, record, err := s.authorizedForMove(ctx, actor, id)
	if err != nil {
		return nil, err
	}

	neighbour, err := Tags(s.db).NewQuery().
		Where("type", "=", record.Type).
		Where("position", comparison, record.Position).
		OrderBy("position", direction).
		First(ctx, g)
	if err != nil {
		return nil, err
	}
	if neighbour == nil {
		return record, nil
	}
	if _, err := security.Authorize(ctx, s.policy, actor, TagUpdate, *neighbour); err != nil {
		return nil, err
	}
	if err := s.swapPositions(ctx, g, record, neighbour); err != nil {
		return nil, err
	}
	return record, nil
}

// authorizedForMove is the opening of every move: decide, load, decide about
// what was loaded.
//
// The second decision is the one a rule about the record depends on. The first
// saw an empty value, so a policy that refuses a label somebody else owns never
// ran against the label being moved.
func (s *TagService) authorizedForMove(ctx context.Context, actor security.Subject, id string) (security.Grant, *Tag, error) {
	g, err := security.Authorize(ctx, s.policy, actor, TagUpdate, Tag{})
	if err != nil {
		return security.Grant{}, nil, err
	}

	record, err := Tags(s.db).NewQuery().WhereKey(id).First(ctx, g)
	if err != nil {
		return security.Grant{}, nil, err
	}
	if record == nil {
		return security.Grant{}, nil, ErrNotFound
	}
	if _, err := security.Authorize(ctx, s.policy, actor, TagUpdate, *record); err != nil {
		return security.Grant{}, nil, err
	}
	return g, record, nil
}

// edgePosition is the lowest or the highest position in use in one taxonomy.
//
// It answers the position of the caller's own record when the taxonomy holds
// nothing else, so a caller comparing the two learns that its record is already
// at that edge.
func (s *TagService) edgePosition(ctx context.Context, g security.Grant, taxonomy, direction string) (int64, error) {
	edge, err := Tags(s.db).NewQuery().
		Where("type", "=", taxonomy).
		OrderBy("position", direction).
		First(ctx, g)
	if err != nil {
		return 0, err
	}
	if edge == nil {
		return 0, ErrNotFound
	}
	return edge.Position, nil
}

// writePosition moves one row, and only while it still holds the position that
// was read.
//
// The condition is what makes a move safe next to another one. Without it a
// caller that read a position, waited, and wrote would move a row it no longer
// knows the place of, and the row it displaced would be displaced twice.
func (s *TagService) writePosition(ctx context.Context, g security.Grant, record *Tag, position int64) error {
	if record.Position == position {
		return nil
	}
	changed, err := Tags(s.db).NewQuery().
		WhereKey(record.ID).
		Where("position", "=", record.Position).
		Update(ctx, g, map[string]any{"position": position})
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("%w: %s was at %d and is not any more", ErrOrderChanged, record.ID, record.Position)
	}
	record.Position = position
	return nil
}

// swapPositions exchanges two rows through a position nothing else can hold.
//
// The taxonomy holds each position once, so the two rows cannot be written past
// each other directly: whichever moves first lands on a place the other still
// occupies. The way through is a third position claimed from the counter, which
// is past everything in use and is never handed out again -- so the first row
// parks there, the second takes the place it left, and the first takes the
// second's.
//
// Each write is conditional on the position that was read, so a move that raced
// another one changes no row and is reported. A failure after the park is
// undone: the parked row is written back to where it started, and when even that
// is lost the row stays at a position past the end of the taxonomy, which is a
// place in the order rather than a duplicate of somebody else's.
func (s *TagService) swapPositions(ctx context.Context, g security.Grant, first, second *Tag) error {
	origin, destination := first.Position, second.Position

	parking, err := s.claimPosition(ctx, g, first.Type)
	if err != nil {
		return err
	}
	if err := s.writePosition(ctx, g, first, parking); err != nil {
		return err
	}
	if err := s.writePosition(ctx, g, second, origin); err != nil {
		if undone := s.writePosition(ctx, g, first, origin); undone == nil {
			return err
		}
		return fmt.Errorf("%w, and %s stayed at %d", err, first.ID, parking)
	}
	return s.writePosition(ctx, g, first, destination)
}

// orderedIDs is the identifiers of these labels, in the order they are given.
func orderedIDs(records []*Tag) []string {
	out := make([]string, 0, len(records))
	for _, record := range records {
		if record != nil {
			out = append(out, record.ID)
		}
	}
	return out
}
