package tags

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/validation"
	"github.com/arandu-io/hesape/database"
)

// The colour of a label, against a database: the token the application
// offers is stored, read back and changed, a token it does not offer is
// refused as input, and the migration that adds the column leaves the rows
// already there as labels with no colour.

// colouredService is a service over a migrated database, with the policy
// opened and these colours offered.
func colouredService(t *testing.T, name string, colours ...string) *TagService {
	t.Helper()
	service := openService(t, name)
	service.colours = colours
	return service
}

// rejectedField reports whether err is a rejected input naming field.
func rejectedField(err error, field string) bool {
	var rejected validation.Errors
	if !errors.As(err, &rejected) {
		return false
	}
	_, named := rejected[field]
	return named
}

func TestALabelKeepsTheColourItWasGiven(t *testing.T) {
	t.Parallel()

	service := colouredService(t, "colour-kept", "red", "green")
	ctx := context.Background()
	actor := memberOf("acme")

	created := mustCreate(t, service, actor, CreateRequest{Name: "Urgent", Taxonomy: "status", Colour: "red"})
	if created.Colour != "red" {
		t.Fatalf("the label was created with %q, want red", created.Colour)
	}
	found, err := service.Find(ctx, actor, created.ID)
	if err != nil || found.Colour != "red" {
		t.Fatalf("the label reads back as %+v (%v), want red", found, err)
	}
	if got := NewResource(*found).ToArray()["colour"]; got != "red" {
		t.Fatalf("the resource answers colour %v, want red", got)
	}

	// A rename that says nothing about the colour leaves it alone.
	renamed, err := service.Rename(ctx, actor, created.ID, RenameRequest{Name: "Very urgent"})
	if err != nil || renamed.Colour != "red" {
		t.Fatalf("a rename without a colour left %+v (%v), want red kept", renamed, err)
	}

	green := "green"
	recoloured, err := service.Rename(ctx, actor, created.ID, RenameRequest{Name: "Very urgent", Colour: &green})
	if err != nil || recoloured.Colour != "green" {
		t.Fatalf("a rename to green left %+v (%v)", recoloured, err)
	}

	none := ""
	cleared, err := service.Rename(ctx, actor, created.ID, RenameRequest{Name: "Very urgent", Colour: &none})
	if err != nil || cleared.Colour != "" {
		t.Fatalf("a rename to no colour left %+v (%v)", cleared, err)
	}
	if found, _ := service.Find(ctx, actor, created.ID); found == nil || found.Colour != "" {
		t.Fatalf("the cleared colour reads back as %+v", found)
	}

	plain := mustCreate(t, service, actor, CreateRequest{Name: "Plain", Taxonomy: "status"})
	if plain.Colour != "" {
		t.Fatalf("a label created with no colour has %q", plain.Colour)
	}
}

func TestAColourTheApplicationDoesNotOfferIsRefused(t *testing.T) {
	t.Parallel()

	service := colouredService(t, "colour-refused", "red")
	ctx := context.Background()
	actor := memberOf("acme")

	for _, colour := range []string{"blue", "Red", "red; background: url(x)", "1red"} {
		if _, err := service.Create(ctx, actor, CreateRequest{Name: "One " + colour, Colour: colour}); !rejectedField(err, "colour") {
			t.Errorf("creating with colour %q returned %v, want the colour rejected", colour, err)
		}
	}

	record := mustCreate(t, service, actor, CreateRequest{Name: "Two", Colour: "red"})
	blue := "blue"
	if _, err := service.Rename(ctx, actor, record.ID, RenameRequest{Name: "Two", Colour: &blue}); !rejectedField(err, "colour") {
		t.Errorf("recolouring to blue returned %v, want the colour rejected", err)
	}
	if found, _ := service.Find(ctx, actor, record.ID); found == nil || found.Colour != "red" {
		t.Fatalf("a refused recolour changed the label to %+v", found)
	}

	// A service that offers no colour refuses every one, which is what an
	// application that never configured colours keeps getting.
	plain := openService(t, "colour-none-offered")
	if _, err := plain.Create(ctx, actor, CreateRequest{Name: "Three", Colour: "red"}); !rejectedField(err, "colour") {
		t.Errorf("a service with no colours accepted red: %v", err)
	}
	if _, err := plain.Create(ctx, actor, CreateRequest{Name: "Three"}); err != nil {
		t.Errorf("a service with no colours refused a label with none: %v", err)
	}
}

func TestTheColourMigrationLeavesExistingLabelsWithoutAColour(t *testing.T) {
	t.Parallel()

	const name = "colour-migration"
	ctx := context.Background()
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

	// The schema of v0.5.2, and a label written by it.
	if err := (createTagTables{}).Up(ctx, migrator); err != nil {
		t.Fatalf("creating the tables: %v", err)
	}
	if _, err := raw.ExecContext(ctx,
		"insert into tags (id, tenant_id, type, name, slug, position, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?)",
		"before", "acme", "status", "Before", "before", 0, "2026-10-01 00:00:00", "2026-10-01 00:00:00"); err != nil {
		t.Fatalf("writing a label of the previous schema: %v", err)
	}

	colour := addTagColour{}
	if err := colour.Up(ctx, migrator); err != nil {
		t.Fatalf("applying %s: %v", colour.GetName(), err)
	}
	service := &TagService{db: data.Wrap(raw, data.DialectSQLite), policy: openPolicy{}, colours: []string{"red"}}
	found, err := service.Find(ctx, memberOf("acme"), "before")
	if err != nil {
		t.Fatalf("reading the label written before the column: %v", err)
	}
	if found.Colour != "" {
		t.Fatalf("the label written before the column reads colour %q, want none", found.Colour)
	}
	red := "red"
	if _, err := service.Rename(ctx, memberOf("acme"), "before", RenameRequest{Name: "Before", Colour: &red}); err != nil {
		t.Fatalf("giving the old label a colour: %v", err)
	}

	// And it rolls back, taking the column with it.
	if err := colour.Down(ctx, migrator); err != nil {
		t.Fatalf("rolling back %s: %v", colour.GetName(), err)
	}
	has, err := migrator.Schema().HasColumn(ctx, TagTable, "colour")
	if err != nil {
		t.Fatalf("asking for the column: %v", err)
	}
	if has {
		t.Fatal("the column is still there after the rollback")
	}
	if err := colour.Up(ctx, migrator); err != nil {
		t.Fatalf("applying %s again after the rollback: %v", colour.GetName(), err)
	}
}

func TestAScreenDrawsAColourAsAClass(t *testing.T) {
	t.Parallel()

	m := &Module{cfg: Config{Tenant: "acme", Colours: []string{"red", "green"}}.withDefaults()}
	labels := m.Labels("en")

	rows := m.Rows(labels, []*Tag{{ID: "a", Name: "A", Colour: "red"}, {ID: "b", Name: "B"}})
	if rows[0].ColourClass != "tag--red" || rows[0].Colour != "red" || rows[0].ColourLabel != "red" {
		t.Fatalf("a red label is drawn as %+v", rows[0])
	}
	if rows[1].ColourClass != "" || rows[1].Colour != "" {
		t.Fatalf("a label with no colour is drawn with %+v", rows[1])
	}

	options := m.ColourOptions(labels, "green")
	if len(options) != 2 || options[0].Token != "red" || options[0].Selected || !options[1].Selected || options[1].Class != "tag--green" {
		t.Fatalf("the form offers %+v", options)
	}
	if got := (&Module{cfg: Config{Tenant: "acme"}}).ColourOptions(labels, ""); got != nil {
		t.Fatalf("an application with no colours is offered %+v", got)
	}
}
