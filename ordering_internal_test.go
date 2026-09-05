package tags

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/arandu-io/framework/security"
)

// The ordering strategy, and the defect it exists to avoid.
//
// The reference implementation appends by reading the largest position in use
// and writing one past it. Nothing connects the read to the write, so two
// writers read the same number, both write it, and the order those rows were
// meant to establish has two of them in one place. Here the write is
// conditional on the read: the update matches the counter only while it still
// holds the value the read returned, so a claim that raced another one changes
// no row, learns it lost, and reads again.
//
// The tests below hold three things:
//
//  1. positions come out in order and PositionStep apart, one writer at a time;
//  2. concurrent writers never take the same one;
//  3. the counter ends where the number of claims says it should, which is what
//     says every claim was counted rather than merely distinct.
//
// Removing the condition -- dropping the Where on next_position in
// claimPosition, so the update writes unconditionally -- makes the second and
// third fail. The unique index over tenant, taxonomy and position is what turns
// a duplicate into a refused write rather than a wrong order, so the failure
// arrives as a create that returned an error, and the count that follows says
// how many did.

func TestPositionsAreHandedOutInOrder(t *testing.T) {
	t.Parallel()

	service := openService(t, "ordering-sequential")
	actor := memberOf("acme")

	for i, name := range []string{"First", "Second", "Third"} {
		record := mustCreate(t, service, actor, CreateRequest{Name: name, Taxonomy: "status"})
		if want := int64(i * PositionStep); record.Position != want {
			t.Fatalf("%s took position %d, want %d", name, record.Position, want)
		}
	}

	// Each taxonomy counts on its own, so the first label of the next one
	// starts over rather than continuing where another left off.
	other := mustCreate(t, service, actor, CreateRequest{Name: "First", Taxonomy: "language"})
	if other.Position != 0 {
		t.Fatalf("the first tag of a second taxonomy took position %d, want 0", other.Position)
	}

	page, err := service.List(context.Background(), actor, "status", queryOfEverything())
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if got := names(page); len(got) != 3 || got[0] != "First" || got[1] != "Second" || got[2] != "Third" {
		t.Fatalf("the listing read %v, want the order they were created in", got)
	}
}

func TestConcurrentCreatesNeverShareAPosition(t *testing.T) {
	t.Parallel()

	service := openService(t, "ordering-concurrent")
	actor := memberOf("acme")
	ctx := context.Background()

	// Enough writers that the interleaving is certain rather than lucky. One
	// connection serves them, so each statement runs alone and is handed back
	// between statements -- which is exactly where a claim that read a value it
	// then writes unconditionally loses it.
	const writers = 64

	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	done.Add(writers)

	positions := make([]int64, writers)
	failures := make([]error, writers)

	for i := range writers {
		go func() {
			defer done.Done()
			start.Wait()

			record, err := service.Create(ctx, actor, CreateRequest{
				Name:     "Tag " + string(rune('A'+i%26)) + strconv.Itoa(i),
				Taxonomy: "status",
			})
			if err != nil {
				failures[i] = err
				return
			}
			positions[i] = record.Position
		}()
	}

	start.Done()
	done.Wait()

	for i, err := range failures {
		if err != nil {
			t.Fatalf("writer %d could not create: %v", i, err)
		}
	}

	seen := make(map[int64]int, writers)
	for i, position := range positions {
		if previous, taken := seen[position]; taken {
			t.Fatalf("writers %d and %d both took position %d", previous, i, position)
		}
		seen[position] = i
	}

	// Distinct is not enough on its own: a claim that skipped ahead on every
	// retry would also be distinct. The counter is where every claim was
	// counted, so it ends exactly this far along and no further.
	counter, err := tagSequences(service.db).NewQuery().
		WhereKey(sequenceKey("acme", "status")).
		First(ctx, security.SystemGrant(TagView, "acme"))
	if err != nil || counter == nil {
		t.Fatalf("reading the counter back: %v", err)
	}
	if want := int64(writers * PositionStep); counter.NextPosition != want {
		t.Fatalf("the counter is at %d after %d claims, want %d", counter.NextPosition, writers, want)
	}

	// And the rows agree with the counter: the listing reads every one of them,
	// in one order, with nothing missing.
	page, err := service.List(ctx, actor, "status", queryOfEverything())
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(page) != writers {
		t.Fatalf("the listing read %d tags, want %d", len(page), writers)
	}
	for i := 1; i < len(page); i++ {
		if page[i-1].Position >= page[i].Position {
			t.Fatalf("the listing is not ordered: %d then %d", page[i-1].Position, page[i].Position)
		}
	}
}

func TestATagIsMovedIntoTheRoomBetweenTwoOthers(t *testing.T) {
	t.Parallel()

	service := openService(t, "ordering-move")
	actor := memberOf("acme")
	ctx := context.Background()

	first := mustCreate(t, service, actor, CreateRequest{Name: "First"})
	second := mustCreate(t, service, actor, CreateRequest{Name: "Second"})
	last := mustCreate(t, service, actor, CreateRequest{Name: "Last"})

	// Positions are handed out PositionStep apart, and the room between two of
	// them is what a move uses. Nothing else is renumbered.
	between := (first.Position + second.Position) / 2
	if _, err := service.Move(ctx, actor, last.ID, between); err != nil {
		t.Fatalf("moving between the first two: %v", err)
	}

	page, err := service.List(ctx, actor, DefaultTaxonomy, queryOfEverything())
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if got := names(page); len(got) != 3 || got[0] != "First" || got[1] != "Last" || got[2] != "Second" {
		t.Fatalf("the listing read %v, want First, Last, Second", got)
	}

	// A position another label holds is refused rather than taken, because
	// taking it would put two labels in one place and the order would stop
	// being an order.
	if _, err := service.Move(ctx, actor, last.ID, first.Position); err == nil {
		t.Fatal("a position another tag holds was accepted")
	}
}

func TestASlugIsHeldOnceWithinATaxonomy(t *testing.T) {
	t.Parallel()

	service := openService(t, "ordering-slug")
	actor := memberOf("acme")
	ctx := context.Background()

	mustCreate(t, service, actor, CreateRequest{Name: "Draft", Taxonomy: "status"})

	// The same name, and a different name that reduces to the same slug: both
	// name the label that is already there.
	for _, name := range []string{"Draft", "  draft  ", "DRAFT!"} {
		if _, err := service.Create(ctx, actor, CreateRequest{Name: name, Taxonomy: "status"}); !errors.Is(err, ErrSlugTaken) {
			t.Fatalf("creating %q a second time returned %v, want ErrSlugTaken", name, err)
		}
	}

	// A second taxonomy is a different place, and the same slug is free there.
	if _, err := service.Create(ctx, actor, CreateRequest{Name: "Draft", Taxonomy: "stage"}); err != nil {
		t.Fatalf("the same slug in another taxonomy was refused: %v", err)
	}
}
