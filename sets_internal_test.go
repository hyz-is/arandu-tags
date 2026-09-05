package tags

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// What an entity carries, as a set, against a database.
//
// Attach and Detach answer about one association and report when the world was
// not what the caller thought. Everything here answers a different question --
// make this the set, and tell me what changed -- and the tests are about the
// difference: the same call made twice leaves the same set, and a sync of one
// taxonomy leaves every other alone.

// carriedNames is the labels one entity carries, by name, in the order they are
// answered in.
func carriedNames(t *testing.T, service *TagService, ref OwnerRef) []string {
	t.Helper()

	carried, err := service.TagsOf(context.Background(), memberOf("acme"), ref)
	if err != nil {
		t.Fatalf("reading what %s carries: %v", ref, err)
	}
	return names(carried)
}

// labelled is a taxonomy of three statuses and two languages, and one entity.
func labelled(t *testing.T, name string) (*TagService, map[string]*Tag, OwnerRef) {
	t.Helper()

	service := openService(t, name)
	actor := memberOf("acme")
	held := map[string]*Tag{}
	for _, label := range []string{"Draft", "Review", "Published"} {
		held[label] = mustCreate(t, service, actor, CreateRequest{Name: label, Taxonomy: "status"})
	}
	for _, label := range []string{"English", "Portuguese"} {
		held[label] = mustCreate(t, service, actor, CreateRequest{Name: label, Taxonomy: "language"})
	}
	return service, held, mustRef(t, testOwnerKind, "article-1")
}

func TestAttachingASetTwiceLeavesTheSameSet(t *testing.T) {
	t.Parallel()

	service, held, ref := labelled(t, "sets-attach")
	actor := memberOf("acme")
	ctx := context.Background()
	wanted := []string{held["Draft"].ID, held["English"].ID}

	attached, err := service.AttachTags(ctx, actor, ref, wanted)
	if err != nil {
		t.Fatalf("attaching: %v", err)
	}
	if attached != 2 {
		t.Fatalf("the first attach wrote %d associations, want 2", attached)
	}

	// The same request sent twice leaves the same set and says nothing changed,
	// which is what makes a resubmitted form something other than an error.
	again, err := service.AttachTags(ctx, actor, ref, wanted)
	if err != nil {
		t.Fatalf("attaching the same set again: %v", err)
	}
	if again != 0 {
		t.Fatalf("the second attach wrote %d associations, want none", again)
	}
	if got := carriedNames(t, service, ref); len(got) != 2 {
		t.Fatalf("the entity carries %v", got)
	}

	// And the singular Attach still reports, because it answers a different
	// question: it is about one association the caller believed was absent.
	if err := service.Attach(ctx, actor, ref, held["Draft"].ID); !errors.Is(err, ErrAlreadyAttached) {
		t.Fatalf("attaching one that is already carried returned %v, want ErrAlreadyAttached", err)
	}
}

func TestDetachingWhatIsNotCarriedIsNotAFailure(t *testing.T) {
	t.Parallel()

	service, held, ref := labelled(t, "sets-detach")
	actor := memberOf("acme")
	ctx := context.Background()

	if _, err := service.AttachTags(ctx, actor, ref, []string{held["Draft"].ID}); err != nil {
		t.Fatalf("attaching: %v", err)
	}

	removed, err := service.DetachTags(ctx, actor, ref, []string{held["Draft"].ID, held["Review"].ID})
	if err != nil {
		t.Fatalf("detaching: %v", err)
	}
	if removed != 1 {
		t.Fatalf("detaching removed %d associations, want 1", removed)
	}
	if got := carriedNames(t, service, ref); len(got) != 0 {
		t.Fatalf("the entity still carries %v", got)
	}

	// The singular Detach still reports, for the reason Attach does.
	if err := service.Detach(ctx, actor, ref, held["Draft"].ID); !errors.Is(err, ErrNotAttached) {
		t.Fatalf("detaching one that is not carried returned %v, want ErrNotAttached", err)
	}
}

func TestEverythingIsTakenBackWhenTheEntityGoesAway(t *testing.T) {
	t.Parallel()

	service, held, ref := labelled(t, "sets-detach-all")
	actor := memberOf("acme")
	ctx := context.Background()

	if _, err := service.AttachTags(ctx, actor, ref, []string{held["Draft"].ID, held["English"].ID}); err != nil {
		t.Fatalf("attaching: %v", err)
	}

	// This package cannot notice an application deleting one of its own rows:
	// the table is theirs and there is no hook in Go that fires when somebody
	// else's row goes away. Without this call the associations outlive the
	// entity and answer questions about an identifier that has been reissued.
	removed, err := service.DetachAllTags(ctx, actor, ref)
	if err != nil {
		t.Fatalf("detaching everything: %v", err)
	}
	if removed != 2 {
		t.Fatalf("detaching everything removed %d associations, want 2", removed)
	}
	if got := carriedNames(t, service, ref); len(got) != 0 {
		t.Fatalf("the entity still carries %v", got)
	}
}

func TestASyncOfOneTaxonomyLeavesTheOthersAlone(t *testing.T) {
	t.Parallel()

	service, held, ref := labelled(t, "sets-sync-taxonomy")
	actor := memberOf("acme")
	ctx := context.Background()

	if _, err := service.AttachTags(ctx, actor, ref, []string{held["Draft"].ID, held["English"].ID}); err != nil {
		t.Fatalf("attaching: %v", err)
	}

	// The form that edits an article's statuses knows nothing about its
	// languages, and a sync that detached them would delete data the person
	// never saw.
	attached, detached, err := service.SyncTagsOfTaxonomy(ctx, actor, ref, "status", []string{held["Published"].ID})
	if err != nil {
		t.Fatalf("syncing one taxonomy: %v", err)
	}
	if attached != 1 || detached != 1 {
		t.Fatalf("the sync attached %d and detached %d, want one of each", attached, detached)
	}
	if got := carriedNames(t, service, ref); !slices.Equal(got, []string{"English", "Published"}) {
		t.Fatalf("the entity carries %v, want English and Published", got)
	}

	// A label of another taxonomy in the list is a caller describing a set it
	// cannot mean, and it is refused rather than partly written.
	if _, _, err := service.SyncTagsOfTaxonomy(ctx, actor, ref, "status", []string{held["Portuguese"].ID}); err == nil {
		t.Fatal("a sync of one taxonomy accepted a label of another")
	}
}

func TestASyncAcrossEveryTaxonomyReplacesEverything(t *testing.T) {
	t.Parallel()

	service, held, ref := labelled(t, "sets-sync-all")
	actor := memberOf("acme")
	ctx := context.Background()

	if _, err := service.AttachTags(ctx, actor, ref, []string{held["Draft"].ID, held["English"].ID}); err != nil {
		t.Fatalf("attaching: %v", err)
	}

	attached, detached, err := service.SyncTags(ctx, actor, ref, []string{held["Portuguese"].ID})
	if err != nil {
		t.Fatalf("syncing: %v", err)
	}
	if attached != 1 || detached != 2 {
		t.Fatalf("the sync attached %d and detached %d, want 1 and 2", attached, detached)
	}
	if got := carriedNames(t, service, ref); !slices.Equal(got, []string{"Portuguese"}) {
		t.Fatalf("the entity carries %v, want only Portuguese", got)
	}

	// The empty set is a legitimate answer to "what should this carry", and it
	// means nothing.
	if _, _, err := service.SyncTags(ctx, actor, ref, nil); err != nil {
		t.Fatalf("syncing to nothing: %v", err)
	}
	if got := carriedNames(t, service, ref); len(got) != 0 {
		t.Fatalf("the entity still carries %v", got)
	}
}

func TestWhatOneEntityCarriesIsAnsweredPerTaxonomy(t *testing.T) {
	t.Parallel()

	service, held, ref := labelled(t, "sets-of-taxonomy")
	actor := memberOf("acme")
	ctx := context.Background()

	if _, err := service.AttachTags(ctx, actor, ref, []string{held["Draft"].ID, held["English"].ID}); err != nil {
		t.Fatalf("attaching: %v", err)
	}

	carried, err := service.TagsOfTaxonomy(ctx, actor, ref, "status")
	if err != nil {
		t.Fatalf("reading one taxonomy: %v", err)
	}
	if got := names(carried); !slices.Equal(got, []string{"Draft"}) {
		t.Fatalf("the status taxonomy answered %v", got)
	}

	carries, err := service.HasTag(ctx, actor, ref, held["Draft"].ID)
	if err != nil || !carries {
		t.Fatalf("HasTag answered (%v, %v) for a label the entity carries", carries, err)
	}
	carries, err = service.HasTag(ctx, actor, ref, held["Review"].ID)
	if err != nil || carries {
		t.Fatalf("HasTag answered (%v, %v) for a label the entity does not carry", carries, err)
	}
}

func TestTheEntitiesCarryingSomethingOfATaxonomy(t *testing.T) {
	t.Parallel()

	service, held, _ := labelled(t, "sets-owners-of-taxonomy")
	actor := memberOf("acme")
	ctx := context.Background()

	for entity, label := range map[string]string{
		"article-1": "Draft",
		"article-2": "Published",
		"article-3": "English",
	} {
		if err := service.Attach(ctx, actor, mustRef(t, testOwnerKind, entity), held[label].ID); err != nil {
			t.Fatalf("attaching %s to %s: %v", label, entity, err)
		}
	}

	// The question is about the taxonomy, not about particular labels: every
	// article that has a status, rather than every article that is a draft.
	found, err := service.OwnersWithAnyTagOfTaxonomy(ctx, actor, testOwnerKind, []string{"status"})
	if err != nil {
		t.Fatalf("asking which entities have a status: %v", err)
	}
	if want := []string{"article-1", "article-2"}; !slices.Equal(found, want) {
		t.Fatalf("read %v, want %v", found, want)
	}

	// A taxonomy nothing is in has no answer rather than every answer.
	none, err := service.OwnersWithAnyTagOfTaxonomy(ctx, actor, testOwnerKind, []string{"nowhere"})
	if err != nil {
		t.Fatalf("asking about an empty taxonomy: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("an empty taxonomy answered %v", none)
	}
}

func TestTheLabelsNothingCarries(t *testing.T) {
	t.Parallel()

	service, held, ref := labelled(t, "sets-unused")
	actor := memberOf("acme")
	ctx := context.Background()

	if _, err := service.AttachTags(ctx, actor, ref, []string{held["Draft"].ID}); err != nil {
		t.Fatalf("attaching: %v", err)
	}

	unused, err := service.UnusedTags(ctx, actor, "status")
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if got := names(unused); !slices.Equal(got, []string{"Review", "Published"}) {
		t.Fatalf("the sweep read %v, want the two nothing carries", got)
	}
}

func TestASetOperationNamingALabelOfAnotherTenantIsRefusedWhole(t *testing.T) {
	t.Parallel()

	service := openService(t, "sets-tenant")
	ctx := context.Background()

	theirs := mustCreate(t, service, memberOf("acme"), CreateRequest{Name: "Theirs", Taxonomy: "status"})
	mine := mustCreate(t, service, memberOf("globex"), CreateRequest{Name: "Mine", Taxonomy: "status"})
	ref := mustRef(t, testOwnerKind, "article-1")

	// One of the two labels is another customer's, so the whole call is refused
	// rather than partly written: an association naming a label of another
	// customer is a row that reads correctly and answers questions about
	// somebody else's taxonomy.
	if _, err := service.AttachTags(ctx, memberOf("globex"), ref, []string{mine.ID, theirs.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("attaching across customers returned %v, want ErrNotFound", err)
	}
	if got := carriedNames(t, service, ref); len(got) != 0 {
		t.Fatalf("a refused attach wrote %v", got)
	}
}
