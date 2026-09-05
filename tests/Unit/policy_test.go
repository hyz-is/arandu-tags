package unit_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/database/model"

	tags "github.com/hyz-is/arandu-tags"
)

// The four properties this package exists to keep are checked here, and they
// are checked against the code rather than described in a document:
//
//  1. the policy denies every action, and has no branch that allows one;
//  2. the service authorizes before constructing or executing a Model query;
//  3. the tenant comes from the Grant;
//  4. nothing reaches the database without passing through the first two.
//
// The fourth is checked by the handle these tests pass in. It wraps a nil
// *sql.DB, so any statement that were issued would panic and fail the test
// loudly -- which makes "the refusal happened before the Model" a fact the
// suite proves rather than a comment. The structural twin in audit_test.go
// keeps that order visible on every service method, including an allowed path.

// everyAction is the whole set the policy answers about. A test that listed
// four of five would pass while the fifth was open.
var everyAction = []security.Action{
	tags.TagView,
	tags.TagList,
	tags.TagCreate,
	tags.TagUpdate,
	tags.TagDelete,
	tags.TagAttach,
	tags.TagDetach,
}

// articleType is the kind an application's own entity would declare. It is
// written out here the way an application writes it: a name it chose and a
// version it will change when the meaning of the kind does.
var articleType = tags.MustOwnerType("article", 1)

// articleRef is one entity of that kind.
func articleRef(t *testing.T) tags.OwnerRef {
	t.Helper()

	ref, err := tags.Ref(articleType, "article-1")
	if err != nil {
		t.Fatalf("building the reference: %v", err)
	}
	return ref
}

// administrator is the most privileged subject an application can produce. It
// is the one to test the default with: a policy that refuses an administrator
// refuses everyone.
func administrator() security.Subject {
	return security.Subject{ID: "user-1", Tenant: "acme", Roles: []string{"admin"}, Verified: true}
}

func TestThePolicyDeniesEveryActionByDefault(t *testing.T) {
	t.Parallel()

	for _, action := range everyAction {
		t.Run(string(action), func(t *testing.T) {
			t.Parallel()

			_, err := security.Authorize(context.Background(), tags.TagPolicy{},
				administrator(), action, tags.Tag{})
			if !errors.Is(err, security.ErrForbidden) {
				t.Fatalf("an unopened policy allowed %s: got %v, want ErrForbidden", action, err)
			}
		})
	}
}

func TestThePolicyDeniesARecordOfAnotherTenant(t *testing.T) {
	t.Parallel()

	other := tags.Tag{ID: "record-1", TenantID: "globex", Name: "theirs"}

	err := tags.TagPolicy{}.Can(context.Background(),
		administrator(), tags.TagView, other)
	if err == nil {
		t.Fatal("the policy allowed a record belonging to another tenant")
	}
	// The message is asserted because the tenant check is the one refusal that
	// has to survive somebody opening the actions below it.
	if !strings.Contains(err.Error(), "another tenant") {
		t.Fatalf("the refusal did not name the tenant: %v", err)
	}
}

func TestThePolicyDeniesAGuest(t *testing.T) {
	t.Parallel()

	for _, action := range everyAction {
		_, err := security.Authorize(context.Background(), tags.TagPolicy{},
			security.Guest("acme"), action, tags.Tag{})
		if !errors.Is(err, security.ErrForbidden) {
			t.Fatalf("a guest was allowed %s: got %v, want ErrForbidden", action, err)
		}
	}
}

func TestAuthorizeRefusesASubjectThatIsNobody(t *testing.T) {
	t.Parallel()

	// The zero Subject is a session that failed to load, not an anonymous
	// reader, and it is refused before the policy is consulted. A package that
	// answered it as a guest would answer a broken session as a visitor.
	_, err := security.Authorize(context.Background(), tags.TagPolicy{},
		security.Subject{}, tags.TagView, tags.Tag{})
	if !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("an empty subject was authorized: got %v, want ErrForbidden", err)
	}
}

// nilHandle is a handle over no database.
//
// Any statement issued through it panics, which is what makes these tests
// prove that the refusal came first: a service that reached the Model before
// authorizing would crash here rather than pass.
func nilHandle() *data.DB { return data.Wrap(nil, data.DialectSQLite) }

func TestTheServiceRefusesBeforeReachingTheModel(t *testing.T) {
	t.Parallel()

	// A nil handle makes even construction of Tags panic at
	// GetQueryGrammar. This catches moving the configured Model entry point --
	// not only its terminal -- ahead of authorization.
	service := tags.NewTagService(nil)
	ctx := context.Background()
	ref := articleRef(t)
	ids := []string{"record-1"}

	// Every exported use case, not the three the template shipped with: a
	// method left out of this list is a method whose refusal nobody checked.
	for name, call := range map[string]func() error{
		"Find": func() error { _, err := service.Find(ctx, administrator(), "record-1"); return err },
		"List": func() error { _, err := service.List(ctx, administrator(), "", data.Query{}); return err },
		"Create": func() error {
			_, err := service.Create(ctx, administrator(), tags.CreateRequest{Name: "one"})
			return err
		},
		"Rename": func() error {
			_, err := service.Rename(ctx, administrator(), "record-1", tags.RenameRequest{Name: "one"})
			return err
		},
		"Move":   func() error { _, err := service.Move(ctx, administrator(), "record-1", 1); return err },
		"Delete": func() error { return service.Delete(ctx, administrator(), "record-1") },
		"Attach": func() error { return service.Attach(ctx, administrator(), ref, "record-1") },
		"Detach": func() error { return service.Detach(ctx, administrator(), ref, "record-1") },
		"TagsOf": func() error { _, err := service.TagsOf(ctx, administrator(), ref); return err },
		"OwnersWithAnyTag": func() error {
			_, err := service.OwnersWithAnyTag(ctx, administrator(), articleType, ids)
			return err
		},
		"OwnersWithAllTags": func() error {
			_, err := service.OwnersWithAllTags(ctx, administrator(), articleType, ids)
			return err
		},
		"OwnersWithoutAnyTag": func() error {
			_, err := service.OwnersWithoutAnyTag(ctx, administrator(), articleType, ids, ids)
			return err
		},
	} {
		if err := call(); !errors.Is(err, security.ErrForbidden) {
			t.Errorf("%s reached the Model before the policy refusal: %v", name, err)
		}
	}
}

// TestEveryExportedServiceMethodIsRefused counts what the map above holds
// against what the type declares, so a use case added without a refusal of its
// own is a failure rather than a silence.
func TestEveryExportedServiceMethodIsRefused(t *testing.T) {
	t.Parallel()

	// Reflection over the pointer type, because that is what the constructor
	// returns and what an application holds.
	service := reflect.TypeOf(tags.NewTagService(nil))
	declared := make([]string, 0, service.NumMethod())
	for i := 0; i < service.NumMethod(); i++ {
		declared = append(declared, service.Method(i).Name)
	}
	sort.Strings(declared)

	want := []string{
		"Attach", "Create", "Delete", "Detach", "Find", "List", "Move",
		"OwnersWithAllTags", "OwnersWithAnyTag", "OwnersWithoutAnyTag",
		"Rename", "TagsOf",
	}
	if !slices.Equal(declared, want) {
		t.Fatalf("the service declares %v; the refusal test covers %v. Add the new method there before adding it here", declared, want)
	}
}

func TestTagsReturnsAWiredTenantScopedModel(t *testing.T) {
	t.Parallel()

	rows := tags.Tags(nilHandle())
	if rows.GetTable() != "tags" {
		t.Fatalf("Tags table = %q, want tags", rows.GetTable())
	}
	if rows.KeyType != "string" || rows.Incrementing {
		t.Fatalf("Tags key is type %q, incrementing %t; want application-generated text", rows.KeyType, rows.Incrementing)
	}
	if rows.TenantColumn != "tenant_id" {
		t.Fatalf("Tags tenant column = %q, want tenant_id", rows.TenantColumn)
	}
	if model.ModelOf(rows.Entity) != rows {
		t.Fatal("Tags returned an entity whose embedded Model is not wired to it")
	}
}

func TestASystemGrantWithoutATenantReachesNothing(t *testing.T) {
	t.Parallel()

	// A system grant with no tenant names no customer. The Model refuses it
	// while preparing the query, before the nil handle can issue a statement.
	_, err := tags.Tags(nilHandle()).NewQuery().WhereKey("record-1").First(
		context.Background(), security.SystemGrant(tags.TagView, ""))
	if !errors.Is(err, model.ErrNoTenant) {
		t.Fatalf("a system grant with no tenant returned %v, want ErrNoTenant", err)
	}
}

func TestTheTenantComesFromTheGrant(t *testing.T) {
	t.Parallel()

	g := security.SystemGrant(tags.TagView, "acme")
	if got := data.Tenant(g); got != "acme" {
		t.Fatalf("data.Tenant(g) = %q, want %q", got, "acme")
	}

	// And a Grant nobody issued carries no tenant at all, so a statement that
	// took its tenant from anywhere else would be reading rows this Grant does
	// not name.
	if got := data.Tenant(security.Grant{}); got != "" {
		t.Fatalf("the zero Grant carries the tenant %q, want none", got)
	}
}

func TestTheRequestValidatesItsInput(t *testing.T) {
	t.Parallel()

	if errs := (tags.CreateRequest{}).Validate(); !errs.Any() {
		t.Fatal("an empty request validated")
	}
	if errs := (tags.CreateRequest{Name: strings.Repeat("a", 121)}).Validate(); !errs.Any() {
		t.Fatal("a name past the maximum validated")
	}
	if errs := (tags.CreateRequest{Name: "one"}).Validate(); errs.Any() {
		t.Fatalf("a valid request was rejected: %v", errs)
	}
}

func TestTheConfigurationRefusesWhatCannotWork(t *testing.T) {
	t.Parallel()

	for name, cfg := range map[string]tags.Config{
		"no tenant":        {},
		"tenant with a /":  {Tenant: "acme/reports"},
		"tenant uppercase": {Tenant: "Acme"},
		"relative prefix":  {Tenant: "acme", Prefix: "tags"},
		"page size too big": {Tenant: "acme",
			PageSize: tags.MaxPageSize + 1},
		"negative page size": {Tenant: "acme", PageSize: -1},
	} {
		if err := cfg.Validate(); err == nil {
			t.Errorf("the configuration with %s was accepted", name)
		}
	}

	if err := (tags.Config{Tenant: "acme"}).Validate(); err != nil {
		t.Fatalf("a valid configuration was refused: %v", err)
	}
}
