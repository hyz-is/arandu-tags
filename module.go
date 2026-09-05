// Package tags is an Arandu module: labels an application's entities carry,
// the association between the two, one policy that decides about them, one
// service that owns their Model-first data path, and the routes that reach
// them.
//
// The files are laid out by role rather than by layer, so the whole package
// reads top to bottom:
//
//	module.go      -> registration, routes, handlers and migrations
//	config.go      -> what the application passes in
//	model.go       -> the entities, and what they may answer with
//	owner.go       -> how an entity says which kind it is
//	policy.go      -> who may do what
//	service.go     -> the rules and Model access, after authorization
//	label.go       -> the label of a tag, in a locale
//	views.go       -> the files the application takes ownership of
//
// An application registers it explicitly. There is no service provider, no
// container and no discovery: the wiring is three lines somebody wrote, and
// reading them is how they learn what the application is made of.
//
// # The routes cover the taxonomy, and the associations are a Go call
//
// The three routes list, read and create labels. Nothing here attaches a label
// to an entity over HTTP, and that is deliberate: whether a caller may tag an
// article is a question about the article, and the policy that answers it
// belongs to whoever owns that table. An application attaches from its own
// handler, where it has already asked. A route here would have to take the
// entity's kind and identifier from the request and tag whatever it was given.
package tags

import (
	"context"
	"errors"
	"fmt"
	stdhttp "net/http"
	"strings"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/foundation"
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/framework/validation"
	"github.com/arandu-io/hesape/database/migrations"
	"github.com/arandu-io/hesape/database/schema"
	"github.com/arandu-io/hesape/view"
)

// Module is what the application registers.
//
// It implements foundation.Module, which is Name and Routes and nothing else --
// that pair is the whole public contract between a package and the framework.
//
// It also implements foundation.Migratable, because it owns tables, and
// foundation.Publishable, because it hands view sources to the project. The
// other optional interfaces are declared beside Module in the framework and are
// opted into the same way, by implementing them: Bootable to prepare state at
// boot, Background to run a loop of its own, Schedulable to declare work for
// the scheduler, Health to report on the storage it depends on, Closable to
// give resources back at shutdown.
type Module struct {
	cfg      Config
	svc      *TagService
	sessions *security.SessionStore
}

// Compile-time proof that the module honors the contracts it claims.
var (
	_ foundation.Module      = (*Module)(nil)
	_ foundation.Migratable  = (*Module)(nil)
	_ foundation.Bootable    = (*Module)(nil)
	_ foundation.Publishable = (*Module)(nil)
)

// New returns the module, or the reason it cannot be built.
//
// The collaborators are parameters and not fields somebody fills in afterwards:
// a module that could be registered half-wired is a module whose first request
// is the thing that reports the missing half.
//
// It returns an error rather than panicking or carrying on, because everything
// it refuses is a wiring mistake, and a wiring mistake found at boot costs one
// restart. The same mistake found later is a request that reached a nil handle.
func New(cfg Config, db *data.DB, sessions *security.SessionStore) (*Module, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if db == nil {
		return nil, errors.New("tags: New needs a database handle: this package owns tables, and there is no in-memory mode that would let it start without one")
	}
	if sessions == nil {
		return nil, errors.New("tags: New needs a session store: it is where the subject comes from, and a request with no subject cannot be authorized")
	}
	cfg = cfg.withDefaults()
	return &Module{
		cfg:      cfg,
		svc:      NewTagService(db),
		sessions: sessions,
	}, nil
}

// Service is the Go API of this package, for an application that attaches
// labels from its own handlers.
//
// It is the same service the routes below call. There is one of it, holding one
// database handle, so a label written through a route and a label read through
// a call are the same rows decided by the same policy.
func (m *Module) Service() *TagService { return m.svc }

// Name is the module identifier: a lowercase slug, stable, no spaces.
//
// It is what `aru route:list` groups by and what the route names are prefixed with,
// so changing it changes addresses that other code has already written down.
func (m *Module) Name() string { return "tags" }

// Routes registers the module's routes under the configured prefix.
//
// They are named, so a URL is built from a name rather than written out a
// second time somewhere else -- two spellings of one address disagree, and the
// failure when they do is a link to a 404.
func (m *Module) Routes(r *fhttp.Router) {
	r.Action(stdhttp.MethodGet, m.cfg.Prefix, m.index).Name("tags.index")
	r.Action(stdhttp.MethodGet, m.cfg.Prefix+"/{id}", m.show).Name("tags.show")
	r.Action(stdhttp.MethodPost, m.cfg.Prefix, m.store).Name("tags.store")
}

// PublishCommand is what an application runs to take ownership of the views
// this package offers.
//
// It is spelled out as a constant so that whatever says it -- the refusal
// below, or a message an application writes for its own operators -- says one
// thing. A person who is told two different commands for one job tries both.
//
// The command reads the modules the application registered and writes what each
// one declares, which is why it is the application's command and not this
// package's: nothing outside the application knows which modules it holds.
const PublishCommand = "aru vendor:publish --apply"

// Boot refuses to serve when a view this package renders is not in the binary.
//
// A compiled view registers itself from init(), so by the time anything boots
// the question has one answer already: either the application published the
// files, compiled them and imported the package they became, or it did not.
// Asking here turns "did anybody run the install command" into one refusal at
// start-up that names the views and the command, instead of a 500 on the first
// request that reached one of them -- which is where it used to be answered,
// once per page, to whoever happened to open it.
//
// It also holds the destination. Every file the archive offers has to land
// under the vendor directory named after this module: an archive that reached
// resources/views/home.kyse.go would land on a page the application wrote, and
// what publishes the files writes what the archive says.
func (m *Module) Boot(context.Context) error {
	prefix := viewRoot + "/" + vendorDir + "/" + m.Name() + "/"
	var stray []string
	for _, path := range PublishedPaths() {
		if !strings.HasPrefix(path, prefix) {
			stray = append(stray, path)
		}
	}
	if len(stray) > 0 {
		return fmt.Errorf("tags: %s would be published outside %s, where it lands on a file the application wrote",
			strings.Join(stray, ", "), prefix)
	}

	registered := make(map[string]bool)
	for _, name := range view.Registered() {
		registered[name] = true
	}
	var missing []string
	for _, name := range ViewNames() {
		if !registered[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		// The compiled packages are named because the import is the half of the
		// install nothing else can do: the command writes the sources, the view
		// compiler turns them into Go, and a package the application does not
		// import is not linked at all.
		imports := make([]string, 0, len(ViewPackages()))
		for _, pkg := range ViewPackages() {
			imports = append(imports, "<module path>/"+pkg)
		}
		return fmt.Errorf("tags: no view is registered as %s. Run `%s`, then `aru view:build`, then import %s in bootstrap/app.go",
			strings.Join(missing, ", "), PublishCommand, strings.Join(imports, ", "))
	}
	return nil
}

// Handlers are thin on purpose: read the input, ask the service, answer. No
// rule, database handle or Model construction lives here. A handler that
// reached data directly would skip the service's policy boundary, and the
// layout makes that visible rather than relying on review.

// index answers a page of one taxonomy.
func (m *Module) index(ctx *fhttp.Context) error {
	query := data.Query{
		Sort:   ctx.Query("sort"),
		Cursor: ctx.Query("cursor"),
		Limit:  m.cfg.PageSize,
	}

	records, err := m.svc.List(ctx.Ctx(), m.subject(ctx.Request), ctx.Query("type"), query)
	if err != nil {
		return m.answer(ctx, err)
	}

	// A full page is the only one that can have a successor. A short page is
	// the last one, and offering a cursor for it would be offering a next page
	// that comes back empty.
	cursor := ""
	if len(records) == m.cfg.PageSize {
		cursor = records[len(records)-1].ID
	}
	return ctx.JSON(stdhttp.StatusOK, collectionFromPointers(records, cursor))
}

// show answers one record.
func (m *Module) show(ctx *fhttp.Context) error {
	record, err := m.svc.Find(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("id"))
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resourceFromPointer(record))
}

// store creates one record.
func (m *Module) store(ctx *fhttp.Context) error {
	in := CreateRequest{Name: ctx.Input("name"), Taxonomy: ctx.Input("type")}

	record, err := m.svc.Create(ctx.Ctx(), m.subject(ctx.Request), in)
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusCreated, resourceFromPointer(record))
}

// subject reads who is acting from the session, and from nowhere else.
//
// A request with no readable session is a declared guest and not an empty
// subject. The difference matters: an empty subject is refused before the
// policy is consulted, because it is almost always a session that failed to
// load, and a policy asked about nobody answers about nobody. A guest reaches
// the policy and is refused there, by a rule somebody wrote -- or allowed,
// where the package means to serve a reader who never signed in.
//
// The tenant of that guest is the application's, from configuration. It is the
// one place a tenant does not come from a Grant, and it is because there is no
// Grant yet: everywhere downstream, data.Tenant is what the statements take.
func (m *Module) subject(r *stdhttp.Request) security.Subject {
	sub, err := m.sessions.Load(r.Context(), r)
	if err != nil || sub.ID == "" {
		return security.Guest(m.cfg.Tenant)
	}
	return sub
}

// answer turns what the service refused into something the client can act on.
//
// Four refusals have an answer, and everything else does not. An error this
// package did not expect is returned rather than swallowed: the framework turns
// it into the error page in development and a 500 in production, which is the
// honest outcome. Answering 200 with an empty body is the failure nobody
// debugs.
//
// A refusal is answered with a status and no detail. Telling the client why a
// policy said no is telling them what exists and what does not, one request at
// a time; the reason is in the log, where the person operating the system reads
// it and the person probing it does not.
//
// A taken slug and an unclaimable position both mean the write did not happen,
// and they are answered apart because what the client should do differs: one is
// settled until the name changes, and the other is the same request made again.
func (m *Module) answer(ctx *fhttp.Context, err error) error {
	switch {
	case errors.Is(err, security.ErrForbidden):
		fhttp.Refuse(ctx.Response, ctx.Request, stdhttp.StatusForbidden, "forbidden")
		return nil
	case errors.Is(err, ErrNotFound):
		fhttp.Refuse(ctx.Response, ctx.Request, stdhttp.StatusNotFound, "not found")
		return nil
	case errors.Is(err, ErrSlugTaken):
		fhttp.Refuse(ctx.Response, ctx.Request, stdhttp.StatusConflict, "conflict")
		return nil
	case errors.Is(err, ErrPositionUnavailable):
		fhttp.Refuse(ctx.Response, ctx.Request, stdhttp.StatusServiceUnavailable, "unavailable")
		return nil
	}

	// A rejected input is the answer rather than a failure, and the fields that
	// were rejected are the client's own, so naming them gives nothing away.
	var rejected validation.Errors
	if errors.As(err, &rejected) {
		fhttp.Refuse(ctx.Response, ctx.Request, stdhttp.StatusUnprocessableEntity, rejected.Error())
		return nil
	}
	return err
}

// Migrations declares the schema this module owns.
//
// They are returned in the order their names sort in, which is the order they
// apply in: the name carries the order, and nothing else decides it.
func (m *Module) Migrations() []foundation.Migration {
	return []foundation.Migration{createTagTables{}}
}

// The migration is reversible, and the assertion is here rather than discovered
// at rollback: the migrator tests for Down with a type assertion, so a Down
// with the wrong signature would leave a rollback that silently does nothing.
var _ migrations.ReversibleMigration = createTagTables{}

// createTagTables is the schema this module owns: the labels, the associations
// and the counter that hands out an order.
//
// One migration rather than three, because the three tables are one thing: a
// counter without labels counts nothing, and an association without both ends
// names nothing. An installer that applied one of the three would have a
// package that cannot answer a single call.
type createTagTables struct{ migrations.BaseMigration }

// GetName is the migration's identity, and it carries the order. It is fixed
// once the package is published: changing what an applied name means leaves the
// change missing everywhere it already ran, and nothing says so.
func (createTagTables) GetName() string { return "20260905_0001_create_tag_tables" }

// Up creates the three tables, the indexes their queries read by, and the two
// unique constraints that hold the invariants the code depends on.
//
// The Blueprint spells each column for the engine the migration is running on,
// which is what lets one application develop on a file and deploy on Postgres
// without a second schema.
//
// The timestamps have no database default: the values come from Go.
func (createTagTables) Up(ctx context.Context, conn migrations.Connection) error {
	err := conn.Schema().Create(ctx, TagTable, func(table *schema.Blueprint) {
		table.String("id").Primary()
		table.String("tenant_id")
		// Not nullable, and the taxonomy with no name is the empty string. Two
		// nulls do not compare equal in SQL, so a nullable discriminator would
		// take the untyped labels out of both unique indexes below -- which is
		// where a taxonomy would quietly grow two labels with one slug.
		table.String("type").Default("")
		table.String("name")
		table.String("slug")
		table.BigInteger("position")
		table.Timestamp("created_at")
		table.Timestamp("updated_at")

		// A slug is how a label is addressed, so a taxonomy holds each one
		// once.
		table.Unique([]string{"tenant_id", "type", "slug"}, "tags_tenant_type_slug_uniq")
		// The order is total, and this is where that is true rather than
		// hoped for: the claim that hands out positions is what avoids the
		// collision, and this is what says so if it ever stops working.
		// It is also the index the listing's ORDER BY reads.
		table.Unique([]string{"tenant_id", "type", "position"}, "tags_tenant_type_position_uniq")
	})
	if err != nil {
		return err
	}

	err = conn.Schema().Create(ctx, TaggableTable, func(table *schema.Blueprint) {
		table.String("id").Primary()
		table.String("tenant_id")
		table.String("tag_id")
		table.String("owner_type")
		table.String("owner_id")
		table.Timestamp("created_at")

		// An entity carries a label once. Attaching it twice is a caller that
		// asked twice, not a second association.
		table.Unique([]string{"tenant_id", "tag_id", "owner_type", "owner_id"}, "taggables_tenant_tag_owner_uniq")
		// The labels one entity carries.
		table.Index([]string{"tenant_id", "owner_type", "owner_id"}, "taggables_tenant_owner_idx")
		// The entities carrying one label, and the grouping that counts how
		// many of a set each of them carries.
		table.Index([]string{"tenant_id", "owner_type", "tag_id", "owner_id"}, "taggables_tenant_type_tag_idx")
	})
	if err != nil {
		return err
	}

	// One row per tenant and taxonomy, addressed by the pair, so a claim reads
	// by key. There is no index beside the primary one: nothing queries this
	// table by anything else.
	return conn.Schema().Create(ctx, tagSequenceTable, func(table *schema.Blueprint) {
		table.String("id").Primary()
		table.String("tenant_id")
		table.String("type").Default("")
		table.BigInteger("next_position")
	})
}

// Down drops the tables, which takes their indexes with them.
//
// In the reverse order of Up, so that a rollback reads as the undo of what was
// read going forward.
func (createTagTables) Down(ctx context.Context, conn migrations.Connection) error {
	if err := conn.Schema().DropIfExists(ctx, tagSequenceTable); err != nil {
		return err
	}
	if err := conn.Schema().DropIfExists(ctx, TaggableTable); err != nil {
		return err
	}
	return conn.Schema().DropIfExists(ctx, TagTable)
}
