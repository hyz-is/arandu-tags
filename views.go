package tags

import (
	"embed"
	"io/fs"
	"strconv"
	"strings"

	"github.com/arandu-io/framework/foundation"
	"github.com/arandu-io/hesape/view"
)

// The view sources this package hands to the project that installs it.
//
// They are embedded rather than read off disk because what a module publishes
// is a tree it carries, not a directory it points at: the program that writes
// the files is the application's own binary, running somewhere this repository
// does not exist as files. Whatever is handed over has to already be inside the
// program doing the handing.
//
// The paths inside the archive are the paths the files take in the project, not
// paths of this repository that something has to translate on the way. A view
// sits here at the address it will sit at there, so the publication names
// neither a source directory nor a destination one, and there is no second
// spelling of a destination for the first one to disagree with.
//
//go:embed resources/views
var viewSources embed.FS

// Where a view is written and what it is called.
//
// viewRoot is the directory every path in the archive starts with, and it is
// what is cut off to get the name a view is registered under. viewSuffix is the
// extension: it ends in .go so the build tag on the first line keeps the
// compiler out of a file that is markup below the package clause.
const (
	viewRoot   = "resources/views"
	viewSuffix = ".kyse.go"
)

// compiledRoot is where the view compiler writes the Go it produces, mirroring
// the tree of the sources. It is build output: gitignored, rebuilt on demand,
// and never edited.
const compiledRoot = "storage/framework/views"

// The names the screens are rendered by.
//
// They are constants because each one is written in a handler and derived again
// from a path, and a page rendered by a name nothing registered is a 500 that
// says nothing about which of the two spellings was wrong. ViewNames derives the
// same set from the archive, and a test holds the two together.
const (
	// ViewIndex is the listing of one taxonomy.
	ViewIndex = "vendor.tags.index"
	// ViewEdit is the form that renames one label.
	ViewEdit = "vendor.tags.edit"
	// ViewOrder is the screen a whole taxonomy is rearranged on.
	ViewOrder = "vendor.tags.order"
	// ViewPicker is the fragment an application draws inside a screen of its
	// own, to show and change what one of its entities carries.
	ViewPicker = "vendor.tags.picker"
)

// Compile-time proof that every screen can be drawn inside the application's
// layout. The layout takes its data through this contract at render time, so a
// page that stopped answering it would be a page that renders as a 500 in
// somebody else's application rather than a build failure in this one.
var (
	_ view.Layout = IndexPageData{}
	_ view.Layout = EditPageData{}
	_ view.Layout = OrderPageData{}
)

// Row is one label as a screen draws it.
//
// It is a snapshot and not the entity, for the reason Resource is one: an
// encoder or a template handed the entity draws whatever fields it happens to
// have, including the ones somebody adds later without opening the markup -- and
// TenantID is exactly such a field.
type Row struct {
	// ID is what a form submits and a link addresses.
	ID string
	// Label is what a person reads: the catalogue's line for this label, or the
	// name on the row when nobody has written one.
	Label string
	// Name is what is in the box when the label is being renamed, which is the
	// stored name rather than the translated one -- renaming a translation would
	// write one language into a column every language reads.
	Name string
	// Slug is what other systems have written down. It is shown and never
	// edited.
	Slug string
	// Taxonomy is the taxonomy this label is in, as stored.
	Taxonomy string
	// TaxonomyLabel is that taxonomy as a person reads it.
	TaxonomyLabel string
	// Position is where it sits, as text, because a screen shows it and no
	// screen does arithmetic on it.
	Position string
	// First and Last say whether this row is at an edge of its listing, so the
	// arrows that would do nothing are drawn as doing nothing.
	First bool
	Last  bool
	// Held says whether the entity a picker is drawn for carries this label. It
	// is false everywhere else, because everywhere else there is no entity for
	// it to be about.
	Held bool
}

// IndexPageData is what the listing screen is handed.
type IndexPageData struct {
	view.Page

	// Prefix is where this module answers, so the markup composes its own
	// addresses instead of hard-coding one the configuration can change.
	Prefix string
	// Labels are the sentences this screen draws, resolved for the locale the
	// request asked for.
	Labels Labels
	// Taxonomy is the one being listed, as stored, and Taxonomies is every one
	// this customer holds a label in.
	Taxonomy   string
	Taxonomies []string
	// Search is the term the listing was narrowed by, echoed back into the field
	// so the box still says what is being looked at.
	Search string
	// Rows are the labels, and Next is the cursor of the following page, empty
	// on the last one.
	Rows []Row
	Next string
}

// EditPageData is what the rename screen is handed.
type EditPageData struct {
	view.Page

	Prefix string
	Labels Labels
	// Row is the label being renamed.
	Row Row
}

// OrderPageData is what the ordering screen is handed.
type OrderPageData struct {
	view.Page

	Prefix string
	Labels Labels
	// Taxonomy is the one being rearranged, as stored.
	Taxonomy string
	// Rows are every label of it, in the order they are in.
	Rows []Row
}

// PickerData is what the association fragment is handed.
//
// It is filled by the application and not by this module, and that is the whole
// point of it: whether somebody may tag an article is a question about the
// article, so the handler that answers it is the one that owns the article. What
// this package contributes is the markup and the shape -- Action is the
// application's own route, and the form posts there.
type PickerData struct {
	// Labels are the sentences the fragment draws.
	Labels Labels
	// Token is the CSRF token of the page this fragment is drawn inside.
	Token string
	// Action is where the form posts: a route of the application, never of this
	// module.
	Action string
	// Taxonomy is the taxonomy being shown, as stored.
	Taxonomy string
	// Carried are the labels the entity has, and Available are the ones it could
	// be given.
	Carried   []Row
	Available []Row
}

// row snapshots one label for a screen.
func (m *Module) row(labels Labels, record *Tag) Row {
	if record == nil {
		return Row{}
	}
	return Row{
		ID:            record.ID,
		Label:         labels.Tag(*record),
		Name:          record.Name,
		Slug:          record.Slug,
		Taxonomy:      record.Type,
		TaxonomyLabel: labels.Taxonomy(record.Type),
		Position:      strconv.FormatInt(record.Position, 10),
	}
}

// rows snapshots a listing, and says which of them are at its edges.
//
// The edges are worked out here rather than in the markup, because the markup
// iterates without an index: a screen that had to know it was drawing the first
// row would need a counter the template language does not give it, and the
// answer is data.
func (m *Module) rows(labels Labels, records []*Tag) []Row {
	out := make([]Row, 0, len(records))
	for i, record := range records {
		row := m.row(labels, record)
		row.First = i == 0
		row.Last = i == len(records)-1
		out = append(out, row)
	}
	return out
}

// Rows snapshots labels for a screen an application draws itself.
//
// It is exported because PickerData is filled outside this package: an
// application holding the labels of one of its entities needs the same snapshot
// the module's own screens are drawn from, and writing a second one is writing a
// second answer to what a label looks like.
func (m *Module) Rows(labels Labels, records []*Tag) []Row { return m.rows(labels, records) }

// PickerRows snapshots the labels an entity could carry, saying which of them it
// already does.
//
// The two lists are passed rather than looked up, because the entity is the
// application's: this package can answer which labels exist and which
// identifiers carry one, and the handler that owns the entity is the one that
// has already decided it may ask.
func (m *Module) PickerRows(labels Labels, available, carried []*Tag) []Row {
	held := make(map[string]bool, len(carried))
	for _, record := range carried {
		if record != nil {
			held[record.ID] = true
		}
	}
	rows := m.rows(labels, available)
	for i := range rows {
		rows[i].Held = held[rows[i].ID]
	}
	return rows
}

// FormState is what a kyse input asks for its message and for what was typed.
//
// It exists because the component library asks for FieldError and the page the
// framework carries answers First. One adapter, in one place, rather than the
// same three lines on every screen -- and it is a type rather than a method on
// each page so that a screen added later cannot forget to write it.
type FormState struct{ view.Page }

// FieldError is the first message for an input, and empty for an input nothing
// rejected.
func (f FormState) FieldError(name string) string { return f.First(name) }

// Form is the state the inputs of this screen read.
func (d IndexPageData) Form() FormState { return FormState{Page: d.Page} }

// Form is the state the inputs of this screen read.
func (d EditPageData) Form() FormState { return FormState{Page: d.Page} }

// Form is the state the inputs of this screen read.
func (d OrderPageData) Form() FormState { return FormState{Page: d.Page} }

// vendorDir is the directory an application keeps other people's views in.
//
// It is part of the path in the archive and not something the publication adds,
// which is what makes the archive a literal picture of what lands in the
// project. Two packages with a view called index are two files under two names
// below it, and neither shadows the other or the application's own.
const vendorDir = "vendor"

// Publishes declares the files this package offers, each at the path it takes
// relative to the root of the project.
//
// One tree, under one tag. A publication may carry a page, a component, a
// configuration file, a migration, a catalogue of sentences or an asset, and
// this package offers the first of those and nothing else. Each absence is a
// decision rather than an omission:
//
//   - configuration is the Config struct New is handed, checked by the compiler
//     and validated before the module exists. A file copied into the project
//     beside it would be a second place to say the same thing, and only one of
//     the two could be the one the code reads.
//   - a stylesheet, a script or any other asset is registered with the view
//     layer and served from an address derived from its own bytes. Copying one
//     into the project would put a second copy of those bytes under a second
//     address, and a page can only reference one of them.
//   - translations are overridden by writing the lines the application wants
//     into its own vendor tree, which the catalogue loader already reads. A
//     copy of every line this package ships is a copy that goes stale, and it
//     goes stale without saying so.
//   - migrations are declared and collected, never copied. A copy in the
//     project's own migration directory is found by the runner as well, so one
//     schema change applies twice under two names.
//
// What is left is the markup, and it is here for the reason the others are not:
// it is the one thing the project is expected to edit. A package cannot know
// what a screen should say in a product it has never seen.
func (m *Module) Publishes() []foundation.Publication {
	return []foundation.Publication{{Tag: foundation.PublishView, Files: viewSources}}
}

// PublishedPaths are the files in the archive, each relative to the root of the
// application, sorted.
func PublishedPaths() []string { return append([]string(nil), publishedPaths...) }

// ViewNames are the names the published views are rendered by, sorted.
//
// The name is the path under the view root with its separators turned into
// dots, which is what the view compiler writes into the registration call. It
// is derived from the same archive the publication carries, so a view that was
// renamed cannot keep an old name here.
func ViewNames() []string { return append([]string(nil), viewNames...) }

// ViewPackages are the directories the compiled views land in, each relative to
// the root of the application, sorted and without repeats.
//
// Importing one is what puts its views in the binary: a compiled view calls the
// registry from init(), and a package nothing imports is not linked at all. The
// import is named rather than written into bootstrap/app.go, because one line
// somebody reads beats a file that changed while they were not looking.
func ViewPackages() []string {
	var out []string
	seen := make(map[string]bool)
	for _, path := range publishedPaths {
		dir := compiledRoot + strings.TrimPrefix(path[:strings.LastIndexByte(path, '/')], viewRoot)
		if seen[dir] {
			continue
		}
		seen[dir] = true
		out = append(out, dir)
	}
	return out
}

// The archive, read once at load.
//
// A failure here is a broken binary rather than a condition to recover from:
// the files are compiled in, so either they are all there or the build that
// produced this program was wrong.
var publishedPaths, viewNames = readArchive()

func readArchive() (paths, names []string) {
	err := fs.WalkDir(viewSources, viewRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, viewSuffix) {
			return nil
		}
		paths = append(paths, path)
		names = append(names, viewName(path))
		return nil
	})
	if err != nil {
		panic("tags: reading the embedded views: " + err.Error())
	}
	if len(paths) == 0 {
		panic("tags: the embedded view directory holds no view, so every name this package renders would be missing and nothing would say so")
	}
	return paths, names
}

// viewName turns an archive path into the name the view is registered under.
//
//	resources/views/vendor/tags/index.kyse.go -> vendor.tags.index
func viewName(path string) string {
	name := strings.TrimPrefix(strings.TrimPrefix(path, viewRoot), "/")
	name = strings.TrimSuffix(name, viewSuffix)
	return strings.ReplaceAll(name, "/", ".")
}
