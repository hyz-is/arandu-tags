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

The files land under `resources/views/vendor/tags/`, and from that point
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
	_ "your/module/path/storage/framework/views/vendor/tags"
```

Without that import the views are not in the binary, and the module refuses to
boot rather than answering the first request that reaches one of them with a
500. The refusal names the view, the command and the import.

## Configuration

| field | required | meaning |
| --- | --- | --- |
| `Tenant` | yes | the customer a visitor with no session is read as. From the application's configuration, never from the request. |
| `Prefix` | no | where the routes are mounted. Defaults to `/tags`. |
| `PageSize` | no | how many records one page answers with. Defaults to 25, refused above 200. |

`New` returns an error rather than starting half-wired, so a setting that
cannot work fails where it is written instead of on the first request that
needed it.

## Routes

| method | path | name |
| --- | --- | --- |
| `GET` | `/tags` | `tags.index` |
| `GET` | `/tags/{id}` | `tags.show` |
| `POST` | `/tags` | `tags.store` |

`index` and `store` read the taxonomy from `type`; leaving it out is the
taxonomy with no name.

Every one of them is refused until the policy is opened. That is the state the
package ships in, and it is deliberate.

The routes cover the labels and nothing else. Attaching a label to one of your
entities is a Go call on `(*Module).Service()`, because whether a caller may tag
an article is a question about the article, and the policy that answers it is
yours. A route here would take a kind and an identifier from the request and tag
whatever it was handed.

## Open the policy

`policy.go` denies every action and has no branch that allows one. Open what
this package needs, one action at a time, inside the custom block:

```go
	// arandu:begin custom
	if a == TagView && (s.ID == record.ID || s.HasRole("admin")) {
		return nil
	}
	// arandu:end custom
```

What is not written there stays closed, including every action added later.

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

`Detach`, `TagsOf`, `OwnersWithAnyTag`, `OwnersWithAllTags` and
`OwnersWithoutAnyTag` are the rest of it. The last three answer with the
identifiers of your entities, not with your rows: this package has never seen
your table and is not going to guess at its schema.

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
module.go      registration, routes, handlers and migrations
config.go      what the application passes in
model.go       the entities, and what they may answer with
owner.go       how an entity says which kind it is
policy.go      who may do what
service.go     the rules and authorized Model access
label.go       the label of a tag, in a locale
views.go       the files the application takes ownership of
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
