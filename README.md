# Arandu Tags

An Arandu package. It holds labels, records which of an application's entities
carry them, registers its own routes, owns its own tables, and decides for
itself who may reach any of it.

## Install

```bash
go get github.com/hyz-is/arandu-tags
```

## Wire it

An Arandu application registers a module explicitly. There is no service
provider, no container and no discovery, so these are the lines to paste into
`bootstrap/app.go` and there are no others.

The import, with the other module imports:

```go
import (
	tags "github.com/hyz-is/arandu-tags"
)
```

The construction, in `Build`, after the session store exists and before
`k.Register`:

```go
	tagsModule, err := tags.New(tags.Config{
		Tenant: cfg.Auth.Tenant,
		CSRF:   csrf,
		// The rules. Nil is the policy this package ships, which denies
		// everything -- see "Write the policy" below.
		Policy: TagRules{},
	}, db, sessions)
	if err != nil {
		return App{}, err
	}
```

And the registration, inside the `k.Register(...)` call already there:

```go
		tagsModule,
```

Then, once, before the application serves:

```bash
aru migrate
```

This package owns three tables -- `tags`, `taggables` and `tag_sequences` --
which is why the migration step is not optional and why `arandu.mod.toml` says
`migrations = true`.

## Publish the views

This package carries the markup of its own pages and hands it over instead of
rendering it from the inside, because a page you cannot edit is a page that says
the wrong thing in your product.

Look at what would be written, then write it:

```bash
aru vendor:publish --tag=view
aru vendor:publish --tag=view --apply
```

Nothing is written without `--apply`. The preview lists every file as `create`,
`update`, `unchanged` or `conflict`, and running the command a second time
writes nothing. A file changed outside its `arandu:begin custom` markers is
reported as a conflict and left alone; `--force` publishes over one, and even
then what is inside the markers is carried forward.

The files land under `resources/views/modules/tags/`, and from that point
they are yours. Nothing of this package is compiled beside them, so no view name
is registered twice and no rule has to decide which of two files won — the
consequence being that a view of this package that changes later does not reach
a project that already published it.

Two steps are left to you, and they are left to you because a command that
edited `bootstrap/app.go` behind your back is a command whose output nobody can
explain. Compile what was written:

```bash
aru view:build
```

and import the directory it wrote into, with the other imports:

```go
	_ "your/module/path/storage/framework/views/modules/tags"
```

Without that import the views are not in the binary, and the module refuses to
boot rather than answering the first request that reaches one of them with a
500. The refusal names the view, the command and the import.

## Configuration

| field | required | meaning |
| --- | --- | --- |
| `Tenant` | yes | the customer a visitor with no session is read as. From the application's configuration, never from the request. |
| `CSRF` | yes | the issuer of the token every form on these screens carries. |
| `Prefix` | no | where the routes are mounted. Defaults to `/tags`. |
| `PageSize` | no | how many records one page answers with. Defaults to 25, refused above 200. |
| `Policy` | no | the rules. Nil is `TagPolicy`, which denies everything. |
| `Translator` | no | your catalogue, asked before the one this package ships. |

`New` returns an error rather than starting half-wired, so a setting that
cannot work fails where it is written instead of on the first request that
needed it.

## Routes

| method | path | name | what it is |
| --- | --- | --- | --- |
| `GET` | `/tags` | `tags.index` | the listing, with search |
| `POST` | `/tags` | `tags.store` | create one |
| `GET` | `/tags/order` | `tags.order` | the reordering screen |
| `POST` | `/tags/order` | `tags.reorder` | write one order over the taxonomy |
| `GET` | `/tags/{id}` | `tags.show` | the form that renames one |
| `PATCH` | `/tags/{id}` | `tags.update` | rename it |
| `DELETE` | `/tags/{id}` | `tags.destroy` | remove it, and its associations |
| `POST` | `/tags/{id}/move` | `tags.move` | `direction` is `up`, `down`, `start` or `end` |

Each of them answers twice over: a browser gets the screen, and a request that
asked for JSON gets the resource. What decides is `ctx.WantsJSON()`, the
framework's own question, so this package invents no second rule about it — and
htmx is on the screen side of that line, because it swaps markup.

`index`, `order` and `store` read the taxonomy from `type`; leaving it out is
the taxonomy with no name. `index` narrows on `q`.

Every one of them is refused until a policy allows something. That is the state
the package ships in, and it is deliberate.

The routes cover the labels and nothing else. Attaching a label to one of your
entities is a Go call on `(*Module).Service()`, because whether a caller may tag
an article is a question about the article, and the policy that answers it is
yours. A route here would take a kind and an identifier from the request and tag
whatever it was handed.

## Write the policy

`TagPolicy`, the one this package ships, denies every action and has no branch
that allows one. It is what a wiring that says nothing about rules gets.

You cannot edit it — it lives inside a package you installed — so the rules go
in your application and reach the module through `Config.Policy`:

```go
type TagRules struct{}

func (TagRules) Can(_ context.Context, s security.Subject, a security.Action, record tags.Tag) error {
	if record.ID != "" && record.TenantID != s.Tenant {
		return fmt.Errorf("the tag belongs to another tenant")
	}
	if !s.HasRole(string(a)) {
		return fmt.Errorf("the subject does not carry %s", a)
	}
	return nil
}
```

Keep the tenant comparison whatever else you write: every rule under it holds
across customers without it, as soon as two of them have a record with the same
identifier. There is still exactly one policy and exactly one place it is
consulted; what `Config.Policy` changes is who writes it.

The actions are `tag.view`, `tag.list`, `tag.create`, `tag.update`,
`tag.delete`, `tag.attach` and `tag.detach`.

## Declare the kind of the thing you tag

An association names the entity on the other side by a kind its own domain
declares, and never by the Go type of that entity. Declare it once, beside the
entity:

```go
var ArticleType = tags.MustOwnerType("article", 1)
```

A Go type name changes when its package is renamed, moved into `internal/`,
aliased or vendored, and none of those changes touch the rows already stored:
every association would go on carrying the name reflection used to report, and
nothing would say so. The version is there for the same reason pointed the other
way -- when the meaning of a kind changes, bump it, and the associations written
against the old one stay attached to the old one instead of claiming rows they
were never about.

Then tag something:

```go
	ref, err := tags.Ref(ArticleType, article.ID)
	if err != nil {
		return err
	}
	if err := module.Service().Attach(ctx, actor, ref, tagID); err != nil {
		return err
	}
```

`Attach` and `Detach` answer about one association and report when the world was
not what you thought — `ErrAlreadyAttached`, `ErrNotAttached`. `AttachTags`,
`DetachTags`, `SyncTags` and `SyncTagsOfTaxonomy` answer a different question:
make this the set, and say what changed. The same request sent twice through
those leaves the same set, which is what a resubmitted form needs.

`DetachAllTags` is what you call when you delete one of your own rows. This
package cannot notice that: the table is yours, and there is no hook in Go that
fires when somebody else's row goes away.

`TagsOf`, `TagsOfTaxonomy` and `HasTag` read what one entity carries.
`OwnersWithAnyTag`, `OwnersWithAllTags`, `OwnersWithoutAnyTag` and
`OwnersWithAnyTagOfTaxonomy` answer with the identifiers of your entities, not
with your rows: this package has never seen your table and is not going to guess
at its schema.

## Find, order and sweep

`FindByName` takes a name or the slug it reduces to, because both are spellings
of one label. `FindManyByName` resolves a whole form in one statement.
`FindInAnyTaxonomy` answers with every label a word could mean.
`FindOrCreate` answers one label per name, in the order you gave them, writing
only what was missing. `Search` matches on the name, ignoring case, treating
`%` and `_` as characters. `Taxonomies` is what the tabs of a screen are drawn
from.

Positions are handed out `PositionStep` apart, so a label moves into the room
between two others without renumbering anything. `MoveUp`, `MoveDown`,
`MoveToStart`, `MoveToEnd` and `SwapOrder` each move one label; `Move` puts one
at a position you name; `Reorder` writes one order over a whole taxonomy from
the identifiers you give, and refuses a list that is not all of them — a partial
reorder produces an order nobody described.

A position is signed. The first claim of a taxonomy lands on zero, and a label
moved before it lands below zero: the alternative is renumbering the taxonomy to
open room at the front, which is the work sparse positions exist to avoid.

`UnusedTags` is the labels nothing carries. It does not say they are unwanted.

## Screens

Four are published: the listing, the form that renames one label, the
reordering screen, and a picker fragment.

The picker is the one no route of this module answers, for the same reason
nothing here attaches over HTTP. You draw it inside a page of your own, from a
handler that has already asked whether this person may tag this article, and
fill `tags.PickerData` — `Action` is your route. `(*Module).PickerRows` gives you
the rows with the ones already carried marked.

The reordering screen moves labels with buttons and not by dragging. A drag
needs a script served beside the page and a keyboard path written a second time;
four buttons are the keyboard path, and nothing has to be downloaded for them to
work. **This package registers no asset of its own.**

## Sentences

The screens read from a catalogue this package embeds, in `en` and `pt-BR`. It
is not published: a view is meant to be edited, so you take ownership of the
file, and a sentence is meant to keep up with the code that produces it.

To change one, write it in your own catalogue under the same key — your
translator is asked first, and pass it as `Config.Translator`. `tags.Lines(locale)`
is what there is to override, and `tags.Locales()` is what is shipped.

A tag's own label is read from the same group, under a key derived from the row:
`tags.draft`, or `tags.status/draft` when it has a taxonomy. A tag your product
ships gets a sentence there; one a customer invented has none, and the name they
typed is shown. The two families cannot collide, because a slug never holds a
dot and every sentence of this package does.

## Commands

```go
	commands, err := tags.Commands(tags.Deps{
		Service:  tagsModule.Service(),
		Operator: func(tenant string) security.Subject { return operatorFor(tenant) },
	})
```

and append them to the slice your console kernel already dispatches. There is no
discovery: a package that registered itself into your console would be a package
whose commands you cannot enumerate by reading your own repository.

| command | what it does |
| --- | --- |
| `tags:list` | print a taxonomy, or the labels a term matches |
| `tags:create` | create one |
| `tags:rename` | rename one, leaving its slug alone |
| `tags:reorder` | write one order from `--id` repeated |
| `tags:taxonomies` | the taxonomies a customer holds labels in |
| `tags:prune` | report the labels nothing carries; `--apply` deletes them |

Every one of them takes `--tenant`: there is no session at a terminal to say
which customer, so the flag is the only answer and a command without it refuses
rather than guessing. Who the command runs as is `Deps.Operator`, which you
write — a package that minted a subject for itself would be a package that
authorizes itself.

## Example

```bash
go run -tags example ./example
```

It opens SQLite in a temporary directory, migrates, writes a policy, and walks
the whole surface: creating, finding, ordering, attaching, syncing, the
catalogue, and what the policy refuses. It leaves nothing behind. The build tag
is there so that installing this package never compiles it.

## Model-first data path

`Tag` embeds `model.Model[Tag]`, and `Tags(db)` is the one
configured entry point for its table. `TagService` owns `*data.DB` and
follows `validate -> security.Authorize -> Grant -> Model terminal`; handlers
never hold the database or construct a Model.

Create writes `TenantID` from `data.Tenant(g)`. Find authorizes before reading
and again against the row it found. List authorizes before building its scoped,
allowlisted query. The Model keeps its default `tenant_id` scope on every
terminal.

Terminals return `*Tag` and `[]*Tag`. Keep those pointers intact:
copying an embedded Model leaves its `Entity` pointer aimed at the original
allocation. `Resource` and `Collection` are explicit response snapshots and do
not expose tenant or Model internals.

There is no CRUD Repository. Add one only for a complex query, read model,
report, export or raw SQL contract that the common Model path cannot express.

## Layout

```
module.go        registration, routes, handlers and migrations
config.go        what the application passes in, and the route table
model.go         the entities, and what they may answer with
owner.go         how an entity says which kind it is
policy.go        who may do what
service.go       the rules and authorized Model access
lookup.go        finding a label by what somebody typed
associations.go  what one entity carries, as a set
ordering.go      where a label sits, and how it is moved
label.go         the label of a tag, in a locale
translation.go   the sentences this package ships
commands.go      what an operator runs from a terminal
views.go         the screens, and the files the project takes over
```

## What is already correct, and has to stay that way

**The policy denies everything.** There is no permit-all branch to delete
later. The Service calls `security.Authorize` before its first `Tags(db)`
reach, and every Model terminal requires the Grant that call produced.

**Authorization precedes the Model.** The package audit checks that order in
every exported Service method. A Model terminal enforces tenant scope; the
preceding Policy call decides whether the action itself is allowed.

**The tenant comes from `data.Tenant(g)`.** Never from the path, the body, the
query string or a header. The value on the Grant came from the session; a value
that arrived with the request is a value the caller chose.

**`arandu.mod.toml` declares what the package does** — network, filesystem,
exec, migrations — and the suite compares the declaration against what the code
*calls*, not against what it imports: `net/http` is imported by everything with
a route and says nothing. A package that says it makes no outbound calls and
then opens one fails its own tests, and that is the only place the comparison
happens. `aru doctor` audits the application it is run inside and never loads a
dependency, so nothing audits an installed package except the package itself.

## Tests

```bash
go test -race ./...
```

The denial suite constructs the Service with a nil database, so even building
`Tags(nil)` would panic. The structural twin reads the allowed path and
rejects any Service method that reaches the Model before `Authorize`.

## Licence

MIT. See [LICENSE.md](LICENSE.md). Copyright Paulo R. Lima.
