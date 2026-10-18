package tags

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/database/model"
)

// OwnersWithAnyTagCondition, against a table of the application's own.
//
// The question it answers is compiled into somebody else's statement, so the
// only way to know it answers correctly is to put it in one: a table this
// package does not own, a Model over it the way an application declares its
// own, and the condition handed to that Model's WhereExists and
// WhereNotExists. The identifiers it is asked about never travel back to Go.

// testArticle is a row of the application's own table, which this package has
// never seen: an identifier, the tenant the Model scopes by, and a title.
type testArticle struct {
	model.Model

	ID       string `db:"id"`
	TenantID string `db:"tenant_id"`
	Title    string `db:"title"`
}

var testArticleTable = model.NewTable(model.TableSpec{
	Name:         "articles",
	New:          func() model.Entity { return new(testArticle) },
	ManualKey:    true,
	NoTimestamps: true,
})

// articlesOf writes the application's table beside the module's, in the same
// database, with these articles for each tenant.
func articlesOf(t *testing.T, db *data.DB, byTenant map[string][]string) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.Statement(ctx, "create table articles (id varchar(255) primary key, tenant_id varchar(255), title varchar(255))", nil); err != nil {
		t.Fatalf("creating the application's table: %v", err)
	}
	for tenant, ids := range byTenant {
		for _, id := range ids {
			if _, err := db.Insert(ctx, "insert into articles (id, tenant_id, title) values (?, ?, ?)", []any{tenant + ":" + id, tenant, id}); err != nil {
				t.Fatalf("writing %s of %s: %v", id, tenant, err)
			}
		}
	}
}

// grantOf is the Grant an application's own policy would have issued for its
// listing, which is what the application's query runs under.
func grantOf(t *testing.T, tenant string) security.Grant {
	t.Helper()
	g, err := security.Authorize(context.Background(), openPolicy{}, memberOf(tenant), TagList, Tag{})
	if err != nil {
		t.Fatalf("authorizing for %s: %v", tenant, err)
	}
	return g
}

// titles runs the application's query and answers the titles, sorted.
func titles(t *testing.T, q *model.Builder, g security.Grant) []string {
	t.Helper()
	found, err := q.Pluck(context.Background(), g, "title")
	if err != nil {
		t.Fatalf("running the application's query: %v", err)
	}
	return distinct(texts(found))
}

// conditionWorld tags articles of two tenants, each under the identifier the
// application keeps in its own id column, and answers the service and the
// labels of acme by name.
func conditionWorld(t *testing.T, name string) (*TagService, map[string]*Tag) {
	t.Helper()
	service := openService(t, name)
	ctx := context.Background()
	articlesOf(t, service.db, map[string][]string{
		"acme":   {"article-1", "article-2", "article-3", "article-4"},
		"globex": {"article-1"},
	})

	acme := memberOf("acme")
	labels := map[string]*Tag{}
	for _, label := range []string{"go", "web", "draft"} {
		labels[label] = mustCreate(t, service, acme, CreateRequest{Name: label, Taxonomy: "topic"})
	}
	for entity, carried := range map[string][]string{
		"article-1": {"go", "web"},
		"article-2": {"go"},
		"article-3": {"draft"},
	} {
		for _, label := range carried {
			if err := service.Attach(ctx, acme, mustRef(t, testOwnerKind, "acme:"+entity), labels[label].ID); err != nil {
				t.Fatalf("attaching %s to %s: %v", label, entity, err)
			}
		}
	}

	// globex tags its own article-1 with a label of its own, called go as well.
	globex := memberOf("globex")
	theirs := mustCreate(t, service, globex, CreateRequest{Name: "go", Taxonomy: "topic"})
	if err := service.Attach(ctx, globex, mustRef(t, testOwnerKind, "globex:article-1"), theirs.ID); err != nil {
		t.Fatalf("attaching for globex: %v", err)
	}
	return service, labels
}

func TestAConditionFiltersTheApplicationsOwnQuery(t *testing.T) {
	t.Parallel()

	service, labels := conditionWorld(t, "condition-any")
	ctx := context.Background()
	acme := memberOf("acme")
	g := grantOf(t, "acme")

	carrying, err := service.OwnersWithAnyTagCondition(ctx, acme, testOwnerKind, []string{labels["go"].ID, labels["draft"].ID}, "articles.id")
	if err != nil {
		t.Fatalf("building the condition: %v", err)
	}

	got := titles(t, testArticleTable.Query(service.db).WhereExists(carrying), g)
	if want := []string{"article-1", "article-2", "article-3"}; !slices.Equal(got, want) {
		t.Fatalf("articles carrying go or draft = %v, want %v", got, want)
	}

	// The same condition the other way round keeps what carries none of them,
	// including the article nothing here has ever been told about.
	got = titles(t, testArticleTable.Query(service.db).WhereNotExists(carrying), g)
	if want := []string{"article-4"}; !slices.Equal(got, want) {
		t.Fatalf("articles carrying neither go nor draft = %v, want %v", got, want)
	}

	// It composes with the rest of the caller's query rather than replacing it.
	got = titles(t, testArticleTable.Query(service.db).Where("title", "!=", "article-1").WhereExists(carrying), g)
	if want := []string{"article-2", "article-3"}; !slices.Equal(got, want) {
		t.Fatalf("articles other than article-1 carrying go or draft = %v, want %v", got, want)
	}

	// And it agrees with the unbounded question it replaces.
	owners, err := service.OwnersWithAnyTag(ctx, acme, testOwnerKind, []string{labels["go"].ID, labels["draft"].ID})
	if err != nil {
		t.Fatalf("asking OwnersWithAnyTag: %v", err)
	}
	if want := []string{"acme:article-1", "acme:article-2", "acme:article-3"}; !slices.Equal(owners, want) {
		t.Fatalf("OwnersWithAnyTag = %v, want %v", owners, want)
	}
}

func TestAConditionCarriesTheLabelsAndNeverTheEntities(t *testing.T) {
	t.Parallel()

	service, labels := conditionWorld(t, "condition-bounded")
	carrying, err := service.OwnersWithAnyTagCondition(context.Background(), memberOf("acme"), testOwnerKind, []string{labels["go"].ID}, "articles.id")
	if err != nil {
		t.Fatalf("building the condition: %v", err)
	}

	statement := testArticleTable.Query(service.db).WhereExists(carrying).GetQuery()
	sql := strings.ToLower(statement.ToSQL())
	if !strings.Contains(sql, "exists (select 1 from") || !strings.Contains(sql, `"taggables"."owner_id" = "articles"."id"`) {
		t.Fatalf("the condition did not compile to a correlated exists subquery:\n%s", sql)
	}
	// The tenant, the kind and the one label are bound, and no identifier of an
	// entity is: the statement is the same size however many articles carry go.
	for _, binding := range statement.GetBindings() {
		if text, ok := binding.(string); ok && strings.HasPrefix(text, "acme:article") {
			t.Fatalf("the statement binds the entity %q", text)
		}
	}
	if got := len(statement.GetBindings()); got != 3 {
		t.Fatalf("the statement binds %d values (%v), want the tenant, the kind and the label", got, statement.GetBindings())
	}
}

func TestAConditionNeverCrossesTenants(t *testing.T) {
	t.Parallel()

	service, labels := conditionWorld(t, "condition-tenants")
	ctx := context.Background()

	// globex names acme's label. The label is not globex's, so the condition
	// names no label at all and keeps nothing -- not even globex's own article,
	// which carries a label of the same name.
	theirs, err := service.OwnersWithAnyTagCondition(ctx, memberOf("globex"), testOwnerKind, []string{labels["go"].ID}, "articles.id")
	if err != nil {
		t.Fatalf("building globex's condition: %v", err)
	}
	if got := titles(t, testArticleTable.Query(service.db).WhereExists(theirs), grantOf(t, "globex")); len(got) != 0 {
		t.Fatalf("globex's query with acme's label kept %v", got)
	}

	// acme's condition run under globex's Grant crosses nothing either: the
	// subquery carries acme's tenant, and the statement's Grant puts globex's
	// on it as well.
	ours, err := service.OwnersWithAnyTagCondition(ctx, memberOf("acme"), testOwnerKind, []string{labels["go"].ID}, "articles.id")
	if err != nil {
		t.Fatalf("building acme's condition: %v", err)
	}
	if got := titles(t, testArticleTable.Query(service.db).WhereExists(ours), grantOf(t, "globex")); len(got) != 0 {
		t.Fatalf("acme's condition under globex's Grant kept %v", got)
	}
}

func TestAConditionThatCannotBeWrittenIsRefused(t *testing.T) {
	t.Parallel()

	service := openService(t, "condition-refused")
	ctx := context.Background()
	acme := memberOf("acme")

	var noKind OwnerType
	if _, err := service.OwnersWithAnyTagCondition(ctx, acme, noKind, []string{"id"}, "articles.id"); err == nil {
		t.Error("a condition about no kind was accepted")
	}
	if _, err := service.OwnersWithAnyTagCondition(ctx, acme, testOwnerKind, nil, "articles.id"); err == nil {
		t.Error("a condition naming no label was accepted")
	}
	for _, column := range []string{
		"",
		"articles.id; drop table tags",
		"articles.id as x",
		`articles"."id`,
		"a.b.c",
		"1id",
		".id",
		"articles.",
		strings.Repeat("a", MaxOwnerColumnLen+1),
	} {
		if _, err := service.OwnersWithAnyTagCondition(ctx, acme, testOwnerKind, []string{"id"}, column); err == nil {
			t.Errorf("the column reference %q was accepted", column)
		}
	}
	for _, column := range []string{"id", "articles.id", "_owner", "Cases.ID", "case_2.owner_id"} {
		if !validColumnReference(column) {
			t.Errorf("the column reference %q was refused", column)
		}
	}
}
