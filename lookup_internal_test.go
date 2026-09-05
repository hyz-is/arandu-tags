package tags

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/arandu-io/framework/data"
)

// Finding a label by what somebody typed, against a database.
//
// These run against one because every one of them is a claim about a statement:
// which rows a name matches, which a slug matches, what a search does to the
// case of both sides, and what a group by answers. None of it can be shown over
// a handle that wraps nothing.

func TestALabelIsFoundByItsNameOrByItsSlug(t *testing.T) {
	t.Parallel()

	service := openService(t, "lookup-by-name")
	actor := memberOf("acme")
	ctx := context.Background()

	created := mustCreate(t, service, actor, CreateRequest{Name: "In Review", Taxonomy: "status"})

	// The name as it was written, and the slug it reduces to. Both are
	// spellings of one label, and a caller holding either of them cannot always
	// tell which it has.
	for _, typed := range []string{"In Review", "in-review"} {
		found, err := service.FindByName(ctx, actor, "status", typed)
		if err != nil {
			t.Fatalf("looking up %q: %v", typed, err)
		}
		if found.ID != created.ID {
			t.Fatalf("%q found %s, want %s", typed, found.ID, created.ID)
		}
	}

	// A taxonomy that does not hold it is not the same taxonomy.
	if _, err := service.FindByName(ctx, actor, "stage", "In Review"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a label of another taxonomy was found: %v", err)
	}
	if _, err := service.FindByName(ctx, actor, "status", "Nothing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a name nothing holds was found: %v", err)
	}
}

func TestANameThatNamesNoLabelIsRefusedBeforeItIsAStatement(t *testing.T) {
	t.Parallel()

	service := openService(t, "lookup-refusals")
	actor := memberOf("acme")
	ctx := context.Background()

	// Punctuation alone reduces to no slug, so it cannot name a label. Answering
	// "not found" would report on a question that was never one.
	if _, err := service.FindByName(ctx, actor, "status", "!!!"); err == nil {
		t.Fatal("a value that reduces to no slug was accepted")
	}
	if _, err := service.FindManyByName(ctx, actor, "status", make([]string, MaxIDsPerQuery+1)); err == nil {
		t.Fatal("a lookup of more names than the ceiling was accepted")
	}
	if _, err := service.Search(ctx, actor, "status", "a", data.Query{}); err == nil {
		t.Fatal("a one-character search term was accepted")
	}
}

func TestOneNameCanBelongToMoreThanOneTaxonomy(t *testing.T) {
	t.Parallel()

	service := openService(t, "lookup-any-taxonomy")
	actor := memberOf("acme")
	ctx := context.Background()

	mustCreate(t, service, actor, CreateRequest{Name: "Draft", Taxonomy: "status"})
	mustCreate(t, service, actor, CreateRequest{Name: "Draft", Taxonomy: "stage"})

	found, err := service.FindInAnyTaxonomy(ctx, actor, "Draft")
	if err != nil {
		t.Fatalf("looking across taxonomies: %v", err)
	}
	// Both, because one name identifies one label inside a taxonomy and
	// identifies nothing across two. Answering with one of them would be
	// answering a question the caller did not ask.
	if len(found) != 2 {
		t.Fatalf("found %d labels called Draft, want 2: %v", len(found), names(found))
	}
	if found[0].Type == found[1].Type {
		t.Fatalf("both came from %q", found[0].Type)
	}
}

func TestFindOrCreateWritesOnlyWhatIsMissing(t *testing.T) {
	t.Parallel()

	service := openService(t, "lookup-find-or-create")
	actor := memberOf("acme")
	ctx := context.Background()

	existing := mustCreate(t, service, actor, CreateRequest{Name: "Draft", Taxonomy: "status"})

	found, err := service.FindOrCreate(ctx, actor, "status", []string{"Draft", "Published"})
	if err != nil {
		t.Fatalf("finding or creating: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("answered with %d labels, want one per name", len(found))
	}
	// One per name, in the order the names were given, so a caller pairs them up
	// without matching strings a second time.
	if found[0].ID != existing.ID {
		t.Fatalf("Draft was created again as %s instead of reusing %s", found[0].ID, existing.ID)
	}
	if found[1].Name != "Published" {
		t.Fatalf("the second answer is %q, want Published", found[1].Name)
	}

	// Two names that reduce to one slug are one label, and are answered with the
	// same row: creating a second would be creating one nothing can tell from
	// the first.
	same, err := service.FindOrCreate(ctx, actor, "status", []string{"Archived", "archived", "ARCHIVED!"})
	if err != nil {
		t.Fatalf("finding or creating one slug three ways: %v", err)
	}
	if same[0].ID != same[1].ID || same[1].ID != same[2].ID {
		t.Fatalf("three spellings of one slug produced %s, %s and %s", same[0].ID, same[1].ID, same[2].ID)
	}

	page, err := service.List(ctx, actor, "status", queryOfEverything())
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(page) != 3 {
		t.Fatalf("the taxonomy holds %d labels, want 3: %v", len(page), names(page))
	}
}

func TestASearchIgnoresCaseAndTreatsWildcardsAsText(t *testing.T) {
	t.Parallel()

	service := openService(t, "lookup-search")
	actor := memberOf("acme")
	ctx := context.Background()

	mustCreate(t, service, actor, CreateRequest{Name: "In Review", Taxonomy: "status"})
	mustCreate(t, service, actor, CreateRequest{Name: "Reviewed", Taxonomy: "status"})
	mustCreate(t, service, actor, CreateRequest{Name: "Draft", Taxonomy: "status"})
	// A name holding what a LIKE clause reads as "anything", so a search that
	// did not escape it would match every row with any term.
	mustCreate(t, service, actor, CreateRequest{Name: "100% done", Taxonomy: "status"})

	for _, term := range []string{"review", "REVIEW", "ReViEw"} {
		found, err := service.Search(ctx, actor, "status", term, data.Query{})
		if err != nil {
			t.Fatalf("searching for %q: %v", term, err)
		}
		if got := names(found); len(got) != 2 || !slices.Contains(got, "In Review") || !slices.Contains(got, "Reviewed") {
			t.Fatalf("searching for %q read %v", term, got)
		}
	}

	// The per-cent sign is a character in a name, not a wildcard. Unescaped it
	// would match all four rows.
	found, err := service.Search(ctx, actor, "status", "0% d", data.Query{})
	if err != nil {
		t.Fatalf("searching for a wildcard character: %v", err)
	}
	if got := names(found); len(got) != 1 || got[0] != "100% done" {
		t.Fatalf("a term holding a wildcard read %v, want only the label that contains it", got)
	}
}

func TestTheTaxonomiesAreTheOnesThatHoldALabel(t *testing.T) {
	t.Parallel()

	service := openService(t, "lookup-taxonomies")
	actor := memberOf("acme")
	ctx := context.Background()

	mustCreate(t, service, actor, CreateRequest{Name: "Draft", Taxonomy: "status"})
	mustCreate(t, service, actor, CreateRequest{Name: "Published", Taxonomy: "status"})
	mustCreate(t, service, actor, CreateRequest{Name: "Portuguese", Taxonomy: "language"})
	// The taxonomy with no name is a taxonomy like any other, and a listing that
	// dropped it would hide every label created without one.
	mustCreate(t, service, actor, CreateRequest{Name: "Loose"})

	found, err := service.Taxonomies(ctx, actor)
	if err != nil {
		t.Fatalf("reading the taxonomies: %v", err)
	}
	if want := []string{DefaultTaxonomy, "language", "status"}; !slices.Equal(found, want) {
		t.Fatalf("read %q, want %q", found, want)
	}

	// And a customer with none reads none rather than another customer's.
	empty, err := service.Taxonomies(ctx, memberOf("globex"))
	if err != nil {
		t.Fatalf("reading the taxonomies of a customer with no label: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("globex read %q", empty)
	}
}

func TestALabelSaysWhetherItIsTheOneSomebodyTyped(t *testing.T) {
	t.Parallel()

	record := Tag{Name: "In Review", Slug: "in-review"}
	for _, typed := range []string{"In Review", "in-review", "IN REVIEW", "in review"} {
		if !record.Matches(typed) {
			t.Errorf("%q did not match the label it names", typed)
		}
	}
	for _, typed := range []string{"Review", "in-reviewed", ""} {
		if record.Matches(typed) {
			t.Errorf("%q matched a label it does not name", typed)
		}
	}
}
