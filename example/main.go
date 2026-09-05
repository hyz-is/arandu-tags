//go:build example

// Command example wires this package the way an application does, and then
// exercises it.
//
// Run it:
//
//	go run -tags example ./example
//
// It is behind a build tag so that installing this package never compiles it:
// what a program does is not a capability whoever ran `go get` agreed to, and a
// library that carries a main package carries one anyway.
//
// It uses SQLite in a temporary directory and leaves nothing behind, so it needs
// no configuration and no server. What it does not do is serve the screens:
// those are published into a project and compiled there, which takes three
// commands and an application to run them in. The wiring below is the same
// either way -- this stops one step short of Routes, and the README carries the
// rest.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	hdatabase "github.com/arandu-io/hesape/database"

	tags "github.com/hyz-is/arandu-tags"

	// The driver. An application picks its own, and this package never does.
	_ "modernc.org/sqlite"
)

// tenant is the customer this example writes as. An application reads its own
// from configuration, and never from a request.
const tenant = "acme"

// ArticleType is the kind this pretend application declares for its own entity.
//
// It is written by hand and never derived from a Go type. A type name changes
// when its package is renamed, moved or vendored, and none of those changes
// touch the rows already stored: every association would go on naming a kind
// nothing answers to, and nothing would say so.
var ArticleType = tags.MustOwnerType("article", 1)

// TagRules is the policy this pretend application writes.
//
// The package ships one that denies everything, which is the state to install
// in. This is what opening it looks like: the tenant comparison first, because
// every rule under it holds across customers without it, and then whatever the
// application decides -- here, that the subject carries the action as a role,
// which is the shape the permission module hands out.
type TagRules struct{}

// Compile-time proof that the rules answer about this entity and no other.
var _ security.Policy[tags.Tag] = TagRules{}

// Can decides whether the subject may perform the action on the record.
func (TagRules) Can(_ context.Context, s security.Subject, a security.Action, record tags.Tag) error {
	if record.ID != "" && record.TenantID != s.Tenant {
		return fmt.Errorf("the tag belongs to another tenant")
	}
	if !s.HasRole(string(a)) {
		return fmt.Errorf("the subject does not carry %s", a)
	}
	return nil
}

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "example:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	dir, err := os.MkdirTemp("", "arandu-tags-example")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	pool, err := sql.Open("sqlite", filepath.Join(dir, "tags.db"))
	if err != nil {
		return err
	}
	defer func() { _ = pool.Close() }()

	// ---- the wiring, which is what an application copies -------------------
	//
	// Every collaborator is a parameter. There is no container, no provider and
	// no discovery: what this module touches is written here and read here.
	key := []byte("0123456789abcdef0123456789abcdef")
	module, err := tags.New(tags.Config{
		Tenant: tenant,
		CSRF:   security.NewCSRF(key, time.Hour),
		Policy: TagRules{},
	},
		data.Wrap(pool, data.DialectSQLite),
		security.NewSessionStore(key, time.Hour, false, security.NewMemoryBackend()),
	)
	if err != nil {
		return err
	}

	// In an application this is `aru migrate`, run as a pipeline step. Never at
	// boot: with N replicas, N migrations race.
	connection := hdatabase.NewConnection(pool, "", "", map[string]any{
		"driver": string(hdatabase.DialectSQLite),
		"name":   "example",
	})
	for _, migration := range module.Migrations() {
		if err := migration.Up(ctx, hdatabase.ForMigrations(connection)); err != nil {
			return fmt.Errorf("applying %s: %w", migration.GetName(), err)
		}
	}

	// The use cases, which are the same ones every screen and every command
	// call. One service, one database handle, one policy.
	svc := module.Service()

	// Who is acting. In an application this comes off the session; here it is
	// written out, carrying the actions the rules above read.
	editor := security.Subject{
		ID: "user-1", Tenant: tenant, Verified: true,
		Roles: []string{
			string(tags.TagView), string(tags.TagList), string(tags.TagCreate),
			string(tags.TagUpdate), string(tags.TagDelete),
			string(tags.TagAttach), string(tags.TagDetach),
		},
	}
	reader := security.Subject{ID: "user-2", Tenant: tenant, Verified: true, Roles: []string{string(tags.TagList)}}

	// ---- the taxonomy ------------------------------------------------------
	say("create", "labels, each claiming a position rather than computing one")
	statuses, err := svc.FindOrCreate(ctx, editor, "status", []string{"Draft", "In Review", "Published"})
	if err != nil {
		return err
	}
	for _, record := range statuses {
		fmt.Printf("    %-12s %-12s position %d\n", record.Name, record.Slug, record.Position)
	}

	say("find or create", "the same names again, which writes nothing")
	same, err := svc.FindOrCreate(ctx, editor, "status", []string{"Draft", "in-review"})
	if err != nil {
		return err
	}
	fmt.Printf("    Draft is still %s, and the slug found the same row as the name\n", same[0].ID[:8])

	say("slug", "one taxonomy holds each slug once")
	if _, err := svc.Create(ctx, editor, tags.CreateRequest{Name: "  draft  ", Taxonomy: "status"}); err != nil {
		fmt.Printf("    refused: %v\n", err)
	}

	// ---- ordering ----------------------------------------------------------
	say("order", "move a label with its neighbour, or write the whole order")
	if _, err := svc.MoveToStart(ctx, editor, statuses[2].ID); err != nil {
		return err
	}
	if err := printOrder(ctx, svc, editor); err != nil {
		return err
	}

	ordered := []string{statuses[0].ID, statuses[1].ID, statuses[2].ID}
	if _, err := svc.Reorder(ctx, editor, "status", ordered); err != nil {
		return err
	}
	fmt.Println("    and back, in one block claimed from the counter:")
	if err := printOrder(ctx, svc, editor); err != nil {
		return err
	}

	// ---- the association ---------------------------------------------------
	//
	// This package holds one half of it. The other half is the application's
	// table, which this package has never seen and could not read.
	say("attach", "an entity of the application's own kind carries labels")
	article, err := tags.Ref(ArticleType, "article-42")
	if err != nil {
		return err
	}
	attached, err := svc.AttachTags(ctx, editor, article, []string{statuses[0].ID, statuses[1].ID})
	if err != nil {
		return err
	}
	fmt.Printf("    %s carries %d labels\n", article, attached)

	again, err := svc.AttachTags(ctx, editor, article, []string{statuses[0].ID})
	if err != nil {
		return err
	}
	fmt.Printf("    the same request again wrote %d, because the set is already what it asked for\n", again)

	say("sync", "the set a form submits replaces the set that was there")
	added, removed, err := svc.SyncTagsOfTaxonomy(ctx, editor, article, "status", []string{statuses[2].ID})
	if err != nil {
		return err
	}
	fmt.Printf("    attached %d, detached %d\n", added, removed)

	carried, err := svc.TagsOf(ctx, reader, article)
	if err != nil {
		return err
	}
	fmt.Printf("    %s now carries %s\n", article, strings.Join(namesOf(carried), ", "))

	say("ask", "which entities carry what, answered with your identifiers")
	owners, err := svc.OwnersWithAnyTagOfTaxonomy(ctx, reader, ArticleType, []string{"status"})
	if err != nil {
		return err
	}
	fmt.Printf("    entities with a status: %s\n", strings.Join(owners, ", "))

	// ---- the labels a person reads -----------------------------------------
	say("label", "the catalogue answers, and the name on the row is the floor")
	labels := module.Labels("pt-BR")
	fmt.Printf("    the screen's own heading: %s\n", labels.T("screen.index_title"))
	fmt.Printf("    a label nobody translated: %s\n", labels.Tag(*statuses[0]))

	// ---- what the policy refuses -------------------------------------------
	say("refuse", "the reader carries tag.list and nothing else")
	if _, err := svc.Create(ctx, reader, tags.CreateRequest{Name: "Sneaky", Taxonomy: "status"}); err != nil {
		fmt.Printf("    creating: %v\n", err)
	}
	stranger := security.Subject{ID: "user-3", Tenant: "globex", Verified: true, Roles: []string{string(tags.TagView)}}
	if _, err := svc.Find(ctx, stranger, statuses[0].ID); err != nil {
		fmt.Printf("    another customer reading it: %v\n", err)
	}

	// ---- what nothing carries ----------------------------------------------
	say("sweep", "the labels nothing carries, which `tags:prune` reports")
	unused, err := svc.UnusedTags(ctx, editor, "status")
	if err != nil {
		return err
	}
	fmt.Printf("    carried by nothing: %s\n", strings.Join(namesOf(unused), ", "))

	say("done", "nothing was left on disk")
	return nil
}

// printOrder writes the taxonomy in the order it is read in.
func printOrder(ctx context.Context, svc *tags.TagService, actor security.Subject) error {
	page, err := svc.List(ctx, actor, "status", data.Query{Limit: 50})
	if err != nil {
		return err
	}
	for _, record := range page {
		fmt.Printf("    %6d  %s\n", record.Position, record.Name)
	}
	return nil
}

// namesOf is what a line of output prints instead of a slice of pointers.
func namesOf(records []*tags.Tag) []string {
	out := make([]string, 0, len(records))
	for _, record := range records {
		out = append(out, record.Name)
	}
	return out
}

// say heads a section, so the output reads as the story it is.
func say(step, what string) {
	fmt.Printf("\n== %s: %s\n", step, what)
}
