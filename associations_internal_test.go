package tags

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// The association, and the four questions asked about it.
//
// These run against a database because the questions are compiled into
// statements: which entities carry any of these labels, which carry all of
// them, which of the ones I am holding carry none, and which labels does this
// one entity carry. The third of those counts inside the engine, and a count
// that does not compile is a package that does not work -- which is not
// something a test over a handle wrapping nothing would find out.

// taggedWorld sets up three entities carrying overlapping labels, and returns
// the labels by name.
func taggedWorld(t *testing.T, name string) (*TagService, map[string]*Tag) {
	t.Helper()

	service := openService(t, name)
	actor := memberOf("acme")
	ctx := context.Background()

	labels := map[string]*Tag{}
	for _, label := range []string{"go", "web", "draft"} {
		labels[label] = mustCreate(t, service, actor, CreateRequest{Name: label, Taxonomy: "topic"})
	}

	// article-1 carries go and web, article-2 carries go, article-3 carries
	// draft. article-4 carries nothing and is never written about.
	for entity, carried := range map[string][]string{
		"article-1": {"go", "web"},
		"article-2": {"go"},
		"article-3": {"draft"},
	} {
		for _, label := range carried {
			if err := service.Attach(ctx, actor, mustRef(t, testOwnerKind, entity), labels[label].ID); err != nil {
				t.Fatalf("attaching %s to %s: %v", label, entity, err)
			}
		}
	}
	return service, labels
}

func TestTheEntitiesCarryingAnyOfTheseLabels(t *testing.T) {
	t.Parallel()

	service, labels := taggedWorld(t, "associations-any")
	actor := memberOf("acme")
	ctx := context.Background()

	found, err := service.OwnersWithAnyTag(ctx, actor, testOwnerKind, []string{labels["go"].ID, labels["draft"].ID})
	if err != nil {
		t.Fatalf("asking for any: %v", err)
	}
	if want := []string{"article-1", "article-2", "article-3"}; !slices.Equal(found, want) {
		t.Fatalf("any of go, draft = %v, want %v", found, want)
	}

	// An entity carrying two of the labels named is answered once, not twice.
	found, err = service.OwnersWithAnyTag(ctx, actor, testOwnerKind, []string{labels["go"].ID, labels["web"].ID})
	if err != nil {
		t.Fatalf("asking for any: %v", err)
	}
	if want := []string{"article-1", "article-2"}; !slices.Equal(found, want) {
		t.Fatalf("any of go, web = %v, want %v", found, want)
	}
}

func TestTheEntitiesCarryingAllOfTheseLabels(t *testing.T) {
	t.Parallel()

	service, labels := taggedWorld(t, "associations-all")
	actor := memberOf("acme")
	ctx := context.Background()

	found, err := service.OwnersWithAllTags(ctx, actor, testOwnerKind, []string{labels["go"].ID, labels["web"].ID})
	if err != nil {
		t.Fatalf("asking for all: %v", err)
	}
	if want := []string{"article-1"}; !slices.Equal(found, want) {
		t.Fatalf("all of go, web = %v, want %v", found, want)
	}

	// The same label named twice is still one label, so the count it is
	// compared against is the count of distinct ones.
	found, err = service.OwnersWithAllTags(ctx, actor, testOwnerKind, []string{labels["go"].ID, labels["go"].ID})
	if err != nil {
		t.Fatalf("asking for all with a repeat: %v", err)
	}
	if want := []string{"article-1", "article-2"}; !slices.Equal(found, want) {
		t.Fatalf("all of go, go = %v, want %v", found, want)
	}

	// Nothing carries all three.
	found, err = service.OwnersWithAllTags(ctx, actor, testOwnerKind,
		[]string{labels["go"].ID, labels["web"].ID, labels["draft"].ID})
	if err != nil {
		t.Fatalf("asking for all three: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("all of go, web, draft = %v, want nothing", found)
	}
}

func TestTheEntitiesCarryingNoneOfTheseLabels(t *testing.T) {
	t.Parallel()

	service, labels := taggedWorld(t, "associations-none")
	actor := memberOf("acme")
	ctx := context.Background()

	// The candidates are the caller's own page of rows, including one this
	// package has never been told about. That is the point of the parameter:
	// article-4 carries nothing, so nothing here has ever seen it, and it still
	// has to come back.
	candidates := []string{"article-1", "article-2", "article-3", "article-4"}

	found, err := service.OwnersWithoutAnyTag(ctx, actor, testOwnerKind, []string{labels["go"].ID}, candidates)
	if err != nil {
		t.Fatalf("asking for none: %v", err)
	}
	if want := []string{"article-3", "article-4"}; !slices.Equal(found, want) {
		t.Fatalf("none of go = %v, want %v", found, want)
	}

	found, err = service.OwnersWithoutAnyTag(ctx, actor, testOwnerKind,
		[]string{labels["go"].ID, labels["draft"].ID}, candidates)
	if err != nil {
		t.Fatalf("asking for none: %v", err)
	}
	if want := []string{"article-4"}; !slices.Equal(found, want) {
		t.Fatalf("none of go, draft = %v, want %v", found, want)
	}
}

func TestAKindIsWhatSeparatesTwoEntitiesWithOneIdentifier(t *testing.T) {
	t.Parallel()

	service, labels := taggedWorld(t, "associations-kind")
	actor := memberOf("acme")
	ctx := context.Background()

	// A different kind, and an entity of it whose identifier is one already in
	// use by an article. Nothing in the identifier says which is which -- the
	// kind does.
	comment := MustOwnerType("comment", 1)
	if err := service.Attach(ctx, actor, mustRef(t, comment, "article-1"), labels["draft"].ID); err != nil {
		t.Fatalf("attaching to a comment: %v", err)
	}

	articles, err := service.OwnersWithAnyTag(ctx, actor, testOwnerKind, []string{labels["draft"].ID})
	if err != nil {
		t.Fatalf("asking about articles: %v", err)
	}
	if want := []string{"article-3"}; !slices.Equal(articles, want) {
		t.Fatalf("the articles carrying draft = %v, want %v", articles, want)
	}

	comments, err := service.OwnersWithAnyTag(ctx, actor, comment, []string{labels["draft"].ID})
	if err != nil {
		t.Fatalf("asking about comments: %v", err)
	}
	if want := []string{"article-1"}; !slices.Equal(comments, want) {
		t.Fatalf("the comments carrying draft = %v, want %v", comments, want)
	}

	// The next version of a kind is a different kind, so nothing written under
	// the first answers for it.
	next := MustOwnerType("comment", 2)
	later, err := service.OwnersWithAnyTag(ctx, actor, next, []string{labels["draft"].ID})
	if err != nil {
		t.Fatalf("asking about the next version of the kind: %v", err)
	}
	if len(later) != 0 {
		t.Fatalf("the associations of comment.v1 answered for comment.v2: %v", later)
	}
}

func TestTheLabelsOneEntityCarries(t *testing.T) {
	t.Parallel()

	service, _ := taggedWorld(t, "associations-of")
	actor := memberOf("acme")
	ctx := context.Background()

	carried, err := service.TagsOf(ctx, actor, mustRef(t, testOwnerKind, "article-1"))
	if err != nil {
		t.Fatalf("reading the labels of an entity: %v", err)
	}
	// In the order of the taxonomy, which is the order they were created in.
	if want := []string{"go", "web"}; !slices.Equal(names(carried), want) {
		t.Fatalf("article-1 carries %v, want %v", names(carried), want)
	}

	none, err := service.TagsOf(ctx, actor, mustRef(t, testOwnerKind, "article-4"))
	if err != nil {
		t.Fatalf("reading the labels of an untagged entity: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("an entity nothing was attached to carries %v", names(none))
	}
}

func TestAnEntityCarriesALabelOnceAndGivesItBackOnce(t *testing.T) {
	t.Parallel()

	service := openService(t, "associations-attach-twice")
	actor := memberOf("acme")
	ctx := context.Background()

	label := mustCreate(t, service, actor, CreateRequest{Name: "go", Taxonomy: "topic"})
	ref := mustRef(t, testOwnerKind, "article-1")

	if err := service.Attach(ctx, actor, ref, label.ID); err != nil {
		t.Fatalf("attaching: %v", err)
	}
	if err := service.Attach(ctx, actor, ref, label.ID); !errors.Is(err, ErrAlreadyAttached) {
		t.Fatalf("attaching twice returned %v, want ErrAlreadyAttached", err)
	}
	if err := service.Detach(ctx, actor, ref, label.ID); err != nil {
		t.Fatalf("detaching: %v", err)
	}
	if err := service.Detach(ctx, actor, ref, label.ID); !errors.Is(err, ErrNotAttached) {
		t.Fatalf("detaching twice returned %v, want ErrNotAttached", err)
	}
}

func TestDeletingALabelTakesItsAssociationsWithIt(t *testing.T) {
	t.Parallel()

	service, labels := taggedWorld(t, "associations-delete")
	actor := memberOf("acme")
	ctx := context.Background()

	if err := service.Delete(ctx, actor, labels["go"].ID); err != nil {
		t.Fatalf("deleting: %v", err)
	}

	if _, err := service.Find(ctx, actor, labels["go"].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the label is still there: %v", err)
	}
	carried, err := service.TagsOf(ctx, actor, mustRef(t, testOwnerKind, "article-1"))
	if err != nil {
		t.Fatalf("reading what is left: %v", err)
	}
	if want := []string{"web"}; !slices.Equal(names(carried), want) {
		t.Fatalf("article-1 carries %v, want %v", names(carried), want)
	}
	// The other label's associations are untouched: a delete removes the rows
	// of one label and not of the entity they hang off.
	others, err := service.OwnersWithAnyTag(ctx, actor, testOwnerKind, []string{labels["draft"].ID})
	if err != nil || !slices.Equal(others, []string{"article-3"}) {
		t.Fatalf("the other associations are now %v (%v)", others, err)
	}
}

func TestAQueryThatCannotBeAnsweredIsRefusedBeforeItIsAStatement(t *testing.T) {
	t.Parallel()

	service := openService(t, "associations-refused")
	actor := memberOf("acme")
	ctx := context.Background()

	var noKind OwnerType
	tooMany := make([]string, MaxIDsPerQuery+1)
	for i := range tooMany {
		tooMany[i] = "id"
	}

	if _, err := service.OwnersWithAnyTag(ctx, actor, noKind, []string{"id"}); !errors.Is(err, ErrOwnerType) {
		t.Fatalf("a query about no kind returned %v", err)
	}
	if _, err := service.OwnersWithAnyTag(ctx, actor, testOwnerKind, nil); err == nil {
		t.Fatal("a query naming no label was accepted")
	}
	if _, err := service.OwnersWithAnyTag(ctx, actor, testOwnerKind, tooMany); err == nil {
		t.Fatalf("a query naming %d labels was accepted", len(tooMany))
	}
	if _, err := service.OwnersWithoutAnyTag(ctx, actor, testOwnerKind, []string{"id"}, tooMany); err == nil {
		t.Fatalf("a query naming %d candidates was accepted", len(tooMany))
	}
}
