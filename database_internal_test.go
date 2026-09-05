package tags

import (
	"context"
	"database/sql"
	"testing"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/database"

	_ "modernc.org/sqlite"
)

// The tests in this file and beside it run against a database.
//
// The rest of the suite proves that a refusal happens before any statement, and
// it proves it by making a statement impossible: the handle wraps nothing, so a
// query that ran would panic. Two properties cannot be shown that way, because
// both are claims about what a statement does:
//
//  1. a subject of one customer cannot reach the rows of another, which is
//     about the predicate that ends up in the query;
//  2. two labels created at the same moment take different positions, which is
//     about what happens when two writers interleave.
//
// These are internal tests, in the package rather than beside it, for one
// reason: the shipped policy refuses everything, which is the state it has to
// be in when somebody installs this. Reaching the service at all means holding
// a policy that answers yes, and the field that holds one is unexported -- so
// only a test compiled into this package can put a different one there. From
// outside, NewTagService remains the only door and it always installs
// TagPolicy.
//
// The engine is SQLite in memory, opened per test under its own name, with one
// connection. One connection is not there to avoid contention: it is what makes
// the interleaving of the ordering test real. Each statement runs alone and the
// connection goes back to the pool between them, so two goroutines claiming a
// position take turns exactly where a claim is vulnerable -- between reading
// the counter and writing it back.

// openPolicy allows every action and says nothing about tenants.
//
// It says nothing about tenants deliberately. The property under test in
// tenant_isolation_internal_test.go is the filter the statement carries, and a
// test policy that also refused a foreign record would answer for it: the test
// would keep passing with the filter removed, which is the one outcome that
// would make it worthless.
type openPolicy struct{}

// Compile-time proof that the substitute answers the same contract the shipped
// policy does.
var _ security.Policy[Tag] = openPolicy{}

// Can allows everything.
func (openPolicy) Can(context.Context, security.Subject, security.Action, Tag) error { return nil }

// openService is a service over a migrated database, with the policy opened.
func openService(t *testing.T, name string) *TagService {
	t.Helper()
	return &TagService{db: openDatabase(t, name), policy: openPolicy{}}
}

// openDatabase returns a migrated, empty database under its own name.
//
// The name is per test rather than shared, so tests running in parallel do not
// read each other's rows and a failure names one test.
func openDatabase(t *testing.T, name string) *data.DB {
	t.Helper()

	raw, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	raw.SetMaxOpenConns(1)

	migrator := database.ForMigrations(database.NewConnection(raw, "", "", map[string]any{
		"driver": string(database.DialectSQLite),
		"name":   name,
	}))
	// The module's own declaration is applied, so the schema this suite
	// exercises and the schema an installer gets cannot drift apart.
	for _, migration := range (&Module{}).Migrations() {
		if err := migration.Up(context.Background(), migrator); err != nil {
			t.Fatalf("applying %s: %v", migration.GetName(), err)
		}
	}

	return data.Wrap(raw, data.DialectSQLite)
}

// memberOf is a subject belonging to one customer.
func memberOf(tenant string) security.Subject {
	return security.Subject{ID: "user-of-" + tenant, Tenant: tenant, Roles: []string{"member"}, Verified: true}
}

// mustCreate creates a label and fails the test when it cannot.
func mustCreate(t *testing.T, s *TagService, actor security.Subject, in CreateRequest) *Tag {
	t.Helper()

	record, err := s.Create(context.Background(), actor, in)
	if err != nil {
		t.Fatalf("creating %q for %s: %v", in.Name, actor.Tenant, err)
	}
	return record
}

// mustRef builds a reference and fails the test when it cannot.
func mustRef(t *testing.T, kind OwnerType, id string) OwnerRef {
	t.Helper()

	ref, err := Ref(kind, id)
	if err != nil {
		t.Fatalf("building a reference to %s %s: %v", kind, id, err)
	}
	return ref
}

// testOwnerKind is the kind an application would declare for its own entity.
var testOwnerKind = MustOwnerType("article", 1)

// queryOfEverything asks for as much as a listing will ever answer with, so a
// test that expects one row and gets two is not a test that paged.
func queryOfEverything() data.Query { return data.Query{Limit: maxLimit} }
