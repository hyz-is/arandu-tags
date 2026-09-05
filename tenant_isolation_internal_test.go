package tags

import (
	"context"
	"errors"
	"testing"
)

// Tenant isolation, against a database, with the policy opened.
//
// The policy substituted in these tests allows every action and says nothing
// about who owns a row, so the only thing standing between one customer and
// another is the predicate the statement carries. Delete the tenant scope from
// Tags -- set m.TenantColumn to "" in model.go -- and every assertion below
// fails, which is the property they are here to hold.
//
// The shipped policy's own refusal of a foreign record is a separate guarantee
// with a separate test, in tests/Unit/policy_test.go. Two independent things
// keep customers apart, and each one is proved with the other out of the way.

func TestATagOfAnotherTenantIsNotReachable(t *testing.T) {
	t.Parallel()

	service := openService(t, "tenant-isolation-find")
	ctx := context.Background()

	theirs := mustCreate(t, service, memberOf("acme"), CreateRequest{Name: "Theirs", Taxonomy: "status"})

	// The identifier is correct, the action is allowed, and the row exists. The
	// only thing that is wrong is who is asking.
	found, err := service.Find(ctx, memberOf("globex"), theirs.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("reading acme's tag as globex returned (%v, %v), want ErrNotFound", found, err)
	}
	// Not found rather than forbidden, and that is the answer to give: telling
	// globex that the identifier exists somewhere is telling them acme has it.
	if found != nil {
		t.Fatalf("a tag of another tenant came back: %#v", found)
	}
}

func TestAListingNeverCrossesTenants(t *testing.T) {
	t.Parallel()

	service := openService(t, "tenant-isolation-list")
	ctx := context.Background()

	mustCreate(t, service, memberOf("acme"), CreateRequest{Name: "Draft", Taxonomy: "status"})
	mustCreate(t, service, memberOf("acme"), CreateRequest{Name: "Published", Taxonomy: "status"})
	mine := mustCreate(t, service, memberOf("globex"), CreateRequest{Name: "Mine", Taxonomy: "status"})

	page, err := service.List(ctx, memberOf("globex"), "status", queryOfEverything())
	if err != nil {
		t.Fatalf("listing for globex: %v", err)
	}
	if len(page) != 1 {
		t.Fatalf("globex read %d tags, want 1: %v", len(page), names(page))
	}
	if page[0].ID != mine.ID {
		t.Fatalf("globex read %q, want its own %q", page[0].Name, mine.Name)
	}
}

func TestTwoTenantsHoldTheSameSlugAndTheSamePosition(t *testing.T) {
	t.Parallel()

	service := openService(t, "tenant-isolation-slug")

	// The unique indexes are over the tenant as well as the taxonomy, so the
	// second create is not a duplicate. A schema that left the tenant out of
	// them would refuse this, and one customer would be able to work out what
	// another has by watching which names it cannot use.
	acme := mustCreate(t, service, memberOf("acme"), CreateRequest{Name: "Draft", Taxonomy: "status"})
	globex := mustCreate(t, service, memberOf("globex"), CreateRequest{Name: "Draft", Taxonomy: "status"})

	if acme.Slug != globex.Slug {
		t.Fatalf("the same name produced %q and %q", acme.Slug, globex.Slug)
	}
	if acme.Position != globex.Position {
		t.Fatalf("the first tag of each tenant took %d and %d; the counters are per tenant", acme.Position, globex.Position)
	}
	if acme.TenantID != "acme" || globex.TenantID != "globex" {
		t.Fatalf("the rows were written for %q and %q", acme.TenantID, globex.TenantID)
	}
}

func TestAnAssociationOfAnotherTenantIsNotReachable(t *testing.T) {
	t.Parallel()

	service := openService(t, "tenant-isolation-associations")
	ctx := context.Background()

	theirs := mustCreate(t, service, memberOf("acme"), CreateRequest{Name: "Theirs", Taxonomy: "status"})
	ref := mustRef(t, testOwnerKind, "article-1")
	if err := service.Attach(ctx, memberOf("acme"), ref, theirs.ID); err != nil {
		t.Fatalf("attaching for acme: %v", err)
	}

	// The identifiers of the tag and of the entity are both right. Everything
	// that could be guessed has been guessed.
	carried, err := service.TagsOf(ctx, memberOf("globex"), ref)
	if err != nil {
		t.Fatalf("reading the tags of the entity as globex: %v", err)
	}
	if len(carried) != 0 {
		t.Fatalf("globex read %d of acme's associations: %v", len(carried), names(carried))
	}

	owners, err := service.OwnersWithAnyTag(ctx, memberOf("globex"), testOwnerKind, []string{theirs.ID})
	if err != nil {
		t.Fatalf("reading the owners as globex: %v", err)
	}
	if len(owners) != 0 {
		t.Fatalf("globex read the entities acme tagged: %v", owners)
	}

	// Detaching is a write, and a write that reached another customer's row
	// would change it rather than merely show it.
	err = service.Detach(ctx, memberOf("globex"), ref, theirs.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("detaching acme's tag as globex returned %v, want ErrNotFound", err)
	}
	stillCarried, err := service.TagsOf(ctx, memberOf("acme"), ref)
	if err != nil || len(stillCarried) != 1 {
		t.Fatalf("acme's association is now %v (%v); globex removed it", names(stillCarried), err)
	}
}

func TestAWriteTakesItsTenantFromTheGrant(t *testing.T) {
	t.Parallel()

	service := openService(t, "tenant-isolation-write")

	record := mustCreate(t, service, memberOf("globex"), CreateRequest{Name: "Mine"})
	if record.TenantID != "globex" {
		t.Fatalf("the row was written for %q, want globex", record.TenantID)
	}

	// And what a response carries never names it. A tenant identifier in a
	// payload is another customer's name leaving the process.
	body := NewResource(*record).ToArray()
	for key, value := range body {
		if key == "tenant_id" {
			t.Fatalf("the resource answers with the tenant: %v", value)
		}
		if text, ok := value.(string); ok && text == "globex" {
			t.Fatalf("the resource answers with the tenant under %q", key)
		}
	}
}

// names is what a failure prints instead of a slice of pointers.
func names(records []*Tag) []string {
	out := make([]string, 0, len(records))
	for _, record := range records {
		out = append(out, record.Name)
	}
	return out
}
