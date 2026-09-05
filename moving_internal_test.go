package tags

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// Moving a label, against a database.
//
// Every one of these is a claim about what happens between two writes, so none
// of them can be shown any other way. The taxonomy holds each position once, and
// that unique index is what turns a wrong move into a refused write rather than
// an order with two labels in one place -- which is why a failure here arrives
// as an error from a move and not as a listing that looks slightly off.
//
// Removing the condition from writePosition -- dropping the Where on position,
// so a move writes whatever it read -- makes
// TestConcurrentSwapsNeverLoseALabelOrDuplicateAPosition fail: two swaps that
// overlap both write the place the other had, and the unique index refuses one
// of them.

// ordered is the names of a taxonomy in the order it is read in.
func ordered(t *testing.T, service *TagService, taxonomy string) []string {
	t.Helper()

	page, err := service.List(context.Background(), memberOf("acme"), taxonomy, queryOfEverything())
	if err != nil {
		t.Fatalf("listing %q: %v", taxonomy, err)
	}
	return names(page)
}

// threeLabels is a taxonomy of three, in the order they were created.
func threeLabels(t *testing.T, name string) (*TagService, *Tag, *Tag, *Tag) {
	t.Helper()

	service := openService(t, name)
	actor := memberOf("acme")
	return service,
		mustCreate(t, service, actor, CreateRequest{Name: "First", Taxonomy: "status"}),
		mustCreate(t, service, actor, CreateRequest{Name: "Second", Taxonomy: "status"}),
		mustCreate(t, service, actor, CreateRequest{Name: "Third", Taxonomy: "status"})
}

func TestALabelExchangesPlacesWithItsNeighbour(t *testing.T) {
	t.Parallel()

	service, _, second, _ := threeLabels(t, "moving-neighbour")
	actor := memberOf("acme")
	ctx := context.Background()

	if _, err := service.MoveUp(ctx, actor, second.ID); err != nil {
		t.Fatalf("moving up: %v", err)
	}
	if got := ordered(t, service, "status"); got[0] != "Second" || got[1] != "First" || got[2] != "Third" {
		t.Fatalf("after moving up: %v", got)
	}

	if _, err := service.MoveDown(ctx, actor, second.ID); err != nil {
		t.Fatalf("moving down: %v", err)
	}
	if got := ordered(t, service, "status"); got[0] != "First" || got[1] != "Second" || got[2] != "Third" {
		t.Fatalf("after moving back down: %v", got)
	}
}

func TestALabelAtAnEdgeStaysWhereItIs(t *testing.T) {
	t.Parallel()

	service, first, _, third := threeLabels(t, "moving-edges")
	actor := memberOf("acme")
	ctx := context.Background()

	// Nothing is written and nothing is refused. A screen's up arrow at the top
	// of a list is not an error, and reporting one would make every list draw a
	// failure on its first row.
	moved, err := service.MoveUp(ctx, actor, first.ID)
	if err != nil {
		t.Fatalf("moving the first label up: %v", err)
	}
	if moved.Position != first.Position {
		t.Fatalf("the first label moved from %d to %d", first.Position, moved.Position)
	}
	if _, err := service.MoveDown(ctx, actor, third.ID); err != nil {
		t.Fatalf("moving the last label down: %v", err)
	}
	if got := ordered(t, service, "status"); got[0] != "First" || got[2] != "Third" {
		t.Fatalf("the order changed at an edge: %v", got)
	}
}

func TestALabelGoesToEitherEndOfItsTaxonomy(t *testing.T) {
	t.Parallel()

	service, _, _, third := threeLabels(t, "moving-ends")
	actor := memberOf("acme")
	ctx := context.Background()

	moved, err := service.MoveToStart(ctx, actor, third.ID)
	if err != nil {
		t.Fatalf("moving to the start: %v", err)
	}
	// Below zero, because the first claim of a taxonomy lands on zero and a
	// label moved before it has to land somewhere. The alternative is
	// renumbering the taxonomy, which is the work sparse positions exist to
	// avoid.
	if moved.Position >= 0 {
		t.Fatalf("the label moved to the start took position %d, and there was no room above zero", moved.Position)
	}
	if got := ordered(t, service, "status"); got[0] != "Third" || got[1] != "First" || got[2] != "Second" {
		t.Fatalf("after moving to the start: %v", got)
	}

	if _, err := service.MoveToEnd(ctx, actor, third.ID); err != nil {
		t.Fatalf("moving to the end: %v", err)
	}
	if got := ordered(t, service, "status"); got[2] != "Third" {
		t.Fatalf("after moving to the end: %v", got)
	}
}

func TestTwoLabelsExchangePlaces(t *testing.T) {
	t.Parallel()

	service, first, _, third := threeLabels(t, "moving-swap")
	actor := memberOf("acme")
	ctx := context.Background()

	if err := service.SwapOrder(ctx, actor, first.ID, third.ID); err != nil {
		t.Fatalf("swapping: %v", err)
	}
	if got := ordered(t, service, "status"); got[0] != "Third" || got[1] != "Second" || got[2] != "First" {
		t.Fatalf("after swapping the ends: %v", got)
	}

	// One label exchanged with itself is the order it already has.
	if err := service.SwapOrder(ctx, actor, first.ID, first.ID); err != nil {
		t.Fatalf("swapping a label with itself: %v", err)
	}

	// And two taxonomies have no order between them, so the exchange describes
	// nothing and is refused rather than written.
	elsewhere := mustCreate(t, service, actor, CreateRequest{Name: "Other", Taxonomy: "stage"})
	if err := service.SwapOrder(ctx, actor, first.ID, elsewhere.ID); !errors.Is(err, ErrDifferentTaxonomies) {
		t.Fatalf("swapping across taxonomies returned %v, want ErrDifferentTaxonomies", err)
	}
}

func TestAnOrderIsWrittenOverAWholeTaxonomy(t *testing.T) {
	t.Parallel()

	service, first, second, third := threeLabels(t, "moving-reorder")
	actor := memberOf("acme")
	ctx := context.Background()

	written, err := service.Reorder(ctx, actor, "status", []string{third.ID, first.ID, second.ID})
	if err != nil {
		t.Fatalf("reordering: %v", err)
	}
	if len(written) != 3 {
		t.Fatalf("the reorder answered with %d labels", len(written))
	}
	// Contiguous and PositionStep apart, because the block is claimed once
	// rather than one position at a time.
	for i := 1; i < len(written); i++ {
		if written[i].Position-written[i-1].Position != PositionStep {
			t.Fatalf("the positions are %d and %d, which are not one step apart",
				written[i-1].Position, written[i].Position)
		}
	}
	if got := ordered(t, service, "status"); got[0] != "Third" || got[1] != "First" || got[2] != "Second" {
		t.Fatalf("after reordering: %v", got)
	}
}

func TestAPartialOrderIsRefusedRatherThanWritten(t *testing.T) {
	t.Parallel()

	service, first, second, _ := threeLabels(t, "moving-partial")
	actor := memberOf("acme")
	ctx := context.Background()
	before := ordered(t, service, "status")

	for name, ids := range map[string][]string{
		"two of three":     {second.ID, first.ID},
		"none":             {},
		"one named twice":  {first.ID, first.ID, second.ID},
		"a stranger among": {first.ID, second.ID, "record-nobody-has"},
	} {
		if _, err := service.Reorder(ctx, actor, "status", ids); !errors.Is(err, ErrOrderIncomplete) {
			t.Errorf("reordering with %s returned %v, want ErrOrderIncomplete", name, err)
		}
	}
	// And nothing was written on the way to being refused: a partial order is a
	// listing somebody cannot predict, and half of one is worse.
	if got := ordered(t, service, "status"); len(got) != len(before) ||
		got[0] != before[0] || got[1] != before[1] || got[2] != before[2] {
		t.Fatalf("a refused reorder changed the order: %v then %v", before, got)
	}
}

func TestConcurrentSwapsNeverLoseALabelOrDuplicateAPosition(t *testing.T) {
	t.Parallel()

	service := openService(t, "moving-concurrent")
	actor := memberOf("acme")
	ctx := context.Background()

	// Six labels, three disjoint pairs, every pair swapped at once. Disjoint so
	// that a correct implementation has nothing to lose: whatever comes back
	// refused is a swap that raced the counter or the write, and whatever comes
	// back wrong is a position two labels reached.
	const pairs = 3
	held := make([]*Tag, 0, pairs*2)
	for i := range pairs * 2 {
		held = append(held, mustCreate(t, service, actor, CreateRequest{
			Name:     string(rune('A' + i)),
			Taxonomy: "status",
		}))
	}

	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	done.Add(pairs)
	failures := make([]error, pairs)

	for i := range pairs {
		go func() {
			defer done.Done()
			start.Wait()
			failures[i] = service.SwapOrder(ctx, actor, held[i*2].ID, held[i*2+1].ID)
		}()
	}
	start.Done()
	done.Wait()

	for i, err := range failures {
		// A lost race is allowed and says so; anything else is a defect.
		if err != nil && !errors.Is(err, ErrOrderChanged) && !errors.Is(err, ErrPositionUnavailable) {
			t.Fatalf("swap %d failed with %v", i, err)
		}
	}

	page, err := service.List(ctx, actor, "status", queryOfEverything())
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(page) != pairs*2 {
		t.Fatalf("the listing read %d labels, want %d: %v", len(page), pairs*2, names(page))
	}
	seen := make(map[int64]string, len(page))
	for _, record := range page {
		if previous, taken := seen[record.Position]; taken {
			t.Fatalf("%s and %s both hold position %d", previous, record.Name, record.Position)
		}
		seen[record.Position] = record.Name
	}
	for i := 1; i < len(page); i++ {
		if page[i-1].Position >= page[i].Position {
			t.Fatalf("the listing is not ordered: %d then %d", page[i-1].Position, page[i].Position)
		}
	}
}
