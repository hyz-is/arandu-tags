# Upgrade Guide

Every heading below is a tag of this repository. Up to `v0.2.2` this file also
carried two sections describing releases of the package skeleton this repository
was cloned from -- `v0.4.0` and `v0.2.0`, with the entity renamed into them,
describing a publishing migration and a Repository removal that both happened
before `v0.1.0` here. They are gone.

## Unreleased

### Go 1.26.10 is the floor

`go.mod` declares `go 1.26.10`. Every vulnerability an Arandu application
reaches is in the Go standard library (crypto/x509, the HTTP/2 and CONNECT
handling in net/http, html/template, net/textproto and crypto/tls) and is
fixed in Go 1.26.10, so a module that declared less let a build run on a
toolchain that still carries them. Build with Go 1.26.10 or later.

The package compiles against newer Arandu modules, and `arandu.mod.toml` now
declares `framework = ">= 0.66"`:

| | was | is |
|---|---|---|
| `github.com/arandu-io/framework` | `v0.56.0` | `v0.66.0` |
| `github.com/arandu-io/hesape` | `v0.54.0` | `v0.85.0` |

```bash
go get github.com/arandu-io/framework@v0.66.0
go get github.com/arandu-io/hesape@v0.85.0
```

An application below those follows the Framework and Hesape upgrade guides
between the versions it had and these first. No route, migration, action,
tenant rule or published view of this package changes.

### `New` and `(*Module).Routes` read the same and compile unchanged

`apidiff` against `v0.6.0` names two incompatible changes, and neither asks
anything of a caller:

- `New`: its `*security.SessionStore` parameter is now an alias of Hesape's
  `http.SessionStore`, so the signature reads the same and the store an
  application already passes is the same type.
- `(*Module).Routes`: the framework's `http.Router` is now an alias of
  Hesape's controller router, with the same consequence.

The page data the screens draw from -- `IndexPageData`, `EditPageData`,
`OrderPageData` and `FormState` -- gains the fields Hesape's `view.Page` added
(`Messages`, `Impersonating`, `Impersonator`, `ImpersonationActs`,
`LeaveImpersonationURL`). A published view that does not read them renders as
it did.

### `TagPolicy` says which actions write

`TagPolicy` implements `auth.Writes`: `tag.view` and `tag.list` read, and the
Grant `Authorize` issues for either is refused by every write with
`auth.ErrReadGrant`. Nothing this package does writes with such a Grant, so the
routes and the service behave as they did.

An application that passes its own policy in `Config.Policy` should declare the
same, so that a Grant issued for reading a label cannot save, delete or
attach one -- `aru doctor` reports a policy that does not as
`policy-declares-no-writes`. Delegate to the shipped answer rather than
restating it:

```go
func (TagRules) Writes(a security.Action) bool { return tags.TagPolicy{}.Writes(a) }
```

A policy without the method keeps issuing Grants that write, as before.

## v0.6.0

No symbol is removed or renamed, and no route, action, policy decision or
tenant rule changes. Everything in this release is added, and an application
that sets none of it behaves as it did on v0.5.2 -- once the migration below has
run. The one change `apidiff` reports is that three types stop being
comparable, which the next section names.

### `Config`, `Module` and `TagService` are no longer comparable

`apidiff` reports the one incompatibility of this release: `Config` gained a
function field (`Subject`) and a slice field (`Colours`), so `==` no longer
compiles on a `Config`, nor on a `Module` or `TagService` value, which hold one
or a slice of their own. Nothing in this package compares them, and the
applications that install it hold `*Module` and `*TagService`, whose pointers
still compare. Code that compared two `Config` values with `==` compares the
fields it means instead, or uses `reflect.DeepEqual`.

### Run the migration before deploying

This release adds `20261018_0001_add_tag_colour`, which adds the nullable column
`tags.colour`. The `Tag` this release reads and writes has the column, so a
binary of this release against a database that has not run it fails to write a
label:

```sh
aru migrate
```

Run it before the deploy, as for every migration. The column is nullable with
no default, so v0.5.2 keeps working against the migrated database during a
rollout, and every existing label reads as one with no colour.

### Colours

`Config.Colours` is new, and empty means what v0.5.2 did: no colour, no choice
on the forms, and a colour sent to `Create` or `Rename` refused as input on
`colour`. To offer colours, name the tokens and style their classes:

```go
tags.New(tags.Config{
	Tenant:  cfg.Auth.Tenant,
	Policy:  TagRules{},
	Colours: []string{"red", "amber", "green"},
}, db, sessions)
```

```css
.tag--red   { background: var(--color-red-100);   color: var(--color-red-900); }
.tag--amber { background: var(--color-amber-100); color: var(--color-amber-900); }
.tag--green { background: var(--color-green-100); color: var(--color-green-900); }
```

The class is assembled from the token at run time, so the rules have to be in
the application's stylesheet: a utility compiler that scans the markup does not
find them. A token is named for people by the catalogue line
`tags.colour.<token>`, and reads as the token itself until one is written.

Three things change shape without breaking a caller:

- The resource answers a `colour` key beside the others, `""` for a label with
  none.
- `RenameRequest.Colour` is a `*string`. A literal that does not set it leaves
  the colour as it is, so `RenameRequest{Name: name}` renames exactly as before.
- `PATCH /tags/{id}` changes the colour only when the input carries a `colour`
  field. A rename form published before this release sends none.

Republish the views to get the colour on the forms, the listing and the
picker's badges; views published before keep compiling and draw no colour.

### The routes can act as the subject the application builds

`Config.Subject` is new and optional. Nil keeps reading the session store handed
to `New`. An application whose members act inside a selected organization
passes the function its own handlers build that subject with:

```go
tags.New(tags.Config{
	Tenant:  cfg.Auth.Tenant,
	Policy:  TagRules{},
	Subject: organizations.SubjectOf, // func(*http.Request) (security.Subject, error)
}, db, sessions)
```

The routes then read and write under that subject's tenant. Its error is the
answer to the request: `security.ErrForbidden` is a 403 and anything else is
returned to the framework's error page, where the session store's own failure
used to be read as a visitor. With `Config.Subject` set, `New` accepts a nil
session store.

### The picker

Republish it to get the two changes below, and the colour of the carried
badges. A picker published before keeps
compiling against this release and draws what it drew.

```sh
aru vendor:publish --tag=view --apply
aru view:build
```

- `PickerData.IDPrefix` is written in front of every checkbox id, through
  `PickerData.CheckboxID`. A page that draws two pickers gives each its own
  prefix; empty draws the ids of v0.5.2.
- The empty states are told apart. A taxonomy with no label draws the new line
  `screen.picker_nothing_to_choose`; `screen.picker_empty` is drawn when labels
  exist and the entity carries none. An application that overrode
  `screen.picker_empty` in its catalogue keeps its sentence, now drawn where
  its wording is right, and writes the new key if it overrides the others.

If you edited the picker, the lines to change are:

| was | is |
| --- | --- |
| `ID:      "tag-" + row.ID,` | `ID:      .CheckboxID(row),` |
| `@else` with `screen.picker_empty` inside the form | `@if(.NothingToChoose())` with `screen.picker_nothing_to_choose` |
| nothing after the carried badges | `@if(.NoneCarried())` with `screen.picker_empty` |

### Filtering a listing by label

`OwnersWithAnyTag` is unchanged, and its answer is every identifier carrying the
labels. An application that sends that answer back as `id IN (...)` moves to
`OwnersWithAnyTagCondition`, which returns a condition for its own query:

```go
// was
ids, err := svc.OwnersWithAnyTag(ctx, actor, CaseType, tagIDs)
page, err := Cases(db).WhereIn("id", toAny(ids)).Get(ctx, g)

// is
carrying, err := svc.OwnersWithAnyTagCondition(ctx, actor, CaseType, tagIDs, "cases.id")
page, err := Cases(db).WhereExists(carrying).Get(ctx, g)
```

The statement then binds the labels and never the entities, so it does not grow
with the number of tagged rows and does not reach PostgreSQL's limit of 65,535
parameters.

## v0.5.2

No symbol, route, migration, action, policy decision or tenant rule changes.
What changes is what an application compiles against: updating to this release
selects Framework v0.56.0, Hesape v0.54.0 and Kyse v0.34.1.

Read their upgrade guides from the versions the application required before.
The ones that reach a running deployment:

- Framework v0.56.0 puts `APP_NAME` on every request, and the screens of this
  package draw it as the brand. An application that never set `APP_NAME` draws
  `arandu-app`, the framework's default; set it.
- Framework v0.56.0 removes `config.Config.SessionTTL`, and `config.Load` no
  longer reads `SESSION_TTL`. An application that built its session store from
  it builds it with `fw.Session.Lifetime` and writes `SESSION_LIFETIME` in
  minutes.
- Kyse v0.34.0 draws `OneTimeCode` and `Masked` with markup that submits
  without script, and Hesape v0.53.0 mounts it. Styles or tests that targeted
  the old markup follow it.

The views this package publishes are unchanged, so there is nothing to
republish.

## v0.5.1

No symbol is removed or changed, and no route, migration, action, policy
decision or tenant rule changes. The published views do not change, so nothing
is republished.

### `Config.CSRF` is deprecated

The screens take the CSRF token `middleware.CSRFProtect` put on the request,
so the issuer passed in `Config` is not read. Remove the line:

```go
tags.New(tags.Config{
	Tenant: cfg.Auth.Tenant,
	CSRF:   csrf, // remove
}, db, sessions)
```

Keeping it compiles and changes nothing; `staticcheck` reports it as SA1019.

### The routes have to sit behind `CSRFProtect`

The application skeleton mounts it for every route, so a project made from it
has nothing to do. Outside it the screens draw an empty token, and nothing
checks the writes they send either: mount the module's routes behind
`CSRFProtect`.

### The layout links

The brand, sign-in, sign-out and register links are read from the routes named
`home`, `auth.login`, `auth.logout` and `auth.register`. An application that
registered those under other names sees the links empty on these screens, as
it does on its own screens built with `view.New`.

## v0.5.0

No symbol is removed or changed, and no route, migration, action, policy
decision or tenant rule changes. What changes is the markup this package
publishes, and **a project that published the views republishes them**.

### Why

aru v0.69 refuses a value interpolated into an address behind text that does not
yet fix the scheme and the host. Seven attributes of the published views did
that, `{{ .Prefix }}/{{ row.ID }}` and its siblings, so a project that
published them stops at `aru view:build`:

```
resources/views/modules/tags/edit.kyse.go:45: this value is written into "hx-patch" before the scheme and the host of the address are fixed
```

The page data now carries each address whole, and the views write it as one
value.

### Republish

If the views compiled before are still in `storage/framework/views/modules/tags`,
publish over them:

```sh
aru vendor:publish --tag=view --apply
aru view:build
```

A file you never edited is updated. One you edited outside its
`arandu:begin custom` markers is reported as a conflict and left alone; publish
it with `--force`, which keeps what is inside the markers, and bring your other
edits across by hand.

If they are not -- a fresh checkout, or a CI run, with aru v0.69 or later --
`aru vendor:publish` stops on the same refusal, because it compiles the view
layer before it asks the application what to publish. Move the four files out of
`resources/views/modules/tags/` first, keeping a copy of your edits, then run the
two commands above. The import of `storage/framework/views/modules/tags` in
`bootstrap/app.go` stays as it is.

### A view you edited

Each composed address becomes the field that carries it:

| was | is |
| --- | --- |
| `{{ .Prefix }}?type={{ .Row.Taxonomy }}` (rename screen) | `{{ .IndexURL }}` |
| `hx-patch="{{ .Prefix }}/{{ .Row.ID }}"` | `{{ .UpdateURL }}` |
| `hx-delete="{{ .Prefix }}/{{ .Row.ID }}"` | `{{ .DeleteURL }}` |
| `{{ .Prefix }}/order?type={{ .Taxonomy }}` | `{{ .OrderURL }}` |
| `@foreach(.Taxonomies as taxonomy)` with `{{ .Prefix }}?type={{ taxonomy }}` and `.Labels.Taxonomy(taxonomy)` | `@foreach(.TaxonomyLinks as taxonomy)` with `{{ taxonomy.URL }}` and `{{ taxonomy.Label }}` |
| `action="{{ .Prefix }}"`, `method="get"` | `{{ .SearchURL }}` |
| `action="{{ .Prefix }}"`, `method="post"` | `{{ .StoreURL }}` |
| `{{ .Prefix }}/{{ row.ID }}` | `{{ row.URL }}` |
| `@if(.Next != "")` with `{{ .Prefix }}?type={{ .Taxonomy }}&amp;cursor={{ .Next }}` | `@if(.NextURL != "")` with `{{ .NextURL }}` |
| `{{ .Prefix }}?type={{ .Taxonomy }}` (ordering screen) | `{{ .IndexURL }}` |
| `hx-post="{{ .Prefix }}/{{ row.ID }}/move"` | `{{ row.MoveURL }}` |
| `hx-post="{{ .Prefix }}/order"` | `{{ .ReorderURL }}` |

`Prefix`, `Taxonomies` and `Next` are still filled, so a view compiled before
keeps compiling against this release until it is republished. A row an
application snapshots itself with `Rows` or `PickerRows` has empty `URL` and
`MoveURL`; the picker writes neither.

## v0.4.2

No symbol, route, migration, action, policy decision or tenant rule changes.
What changes is what an application compiles against: updating to this release
selects Framework v0.55.1, Hesape v0.52.0 and Kyse v0.33.0.

Those releases can stop an application that booted before from booting, and
this package cannot make that change for it. Read their upgrade guides from the
versions the application required before. The ones that reach a running
deployment:

- Framework v0.55.0 makes `Configuration.Session` a `bootstrap.Session` with
  `Secure` and `Lifetime`, refuses `SESSION_TTL`, and stops the boot on a
  `SESSION_*` variable the session store does not read. An application that
  built its store from `SESSION_TTL` builds it with `fw.Session.Lifetime` and
  writes `SESSION_LIFETIME` in minutes.
- Framework v0.54.0 stops the boot on a boolean setting that does not read as
  one, such as `SESSION_SECURE_COOKIE=sometimes`.
- Hesape v0.52.0 removes the names it deprecated in v0.50.1 and v0.50.2.

The views this package publishes are unchanged, so there is nothing to
republish.

## v0.4.1

Nothing to change in an application: this release touches skills, not Go code.
`aru skills:sync` now offers `tags-package` to a project that requires this
version. An application wired from the earlier copy of that skill never booted,
because it left out `CSRF`; one that booted already passes it.

## v0.4.0

### Published views move out of `vendor/`

The views this package publishes land in `resources/views/modules/tags/` and
compile to `storage/framework/views/modules/tags`. It used to be `vendor/` in
both, and that address could not work: the go command refuses to import a
package whose path carries a `vendor` element —

```
bootstrap/app.go:98:2: use of vendored package not allowed
```

— and a published view is compiled into a Go package the application has to
import for its `init()` to register anything. So the last step of the install,
the import `(*Module).Boot` asks for, did not build.

The archive was already under `resources/publish`, which is what keeps the files
in the module zip: a file under a directory named `vendor` is dropped from it at
any depth. That fixed the source side and left the destination carrying the
word, and the destination is the address the application looks the views up at.

The value of every view name constant moved with it. The constant names are
unchanged, so code that renders through them keeps compiling, and each one now
answers `modules.tags.…` where it answered `vendor.tags.…`:

- `ViewIndex`
- `ViewEdit`
- `ViewOrder`
- `ViewPicker`

A string written out by hand instead of through the constant stops matching, and
what that produces is a 500 saying no view is registered under the old name.

**A project that already published the old tree** publishes again and removes
the old one by hand:

```sh
aru vendor:publish --tag=view --apply
aru view:build
rm -rf resources/views/vendor/tags storage/framework/views/vendor/tags
```

then deletes the old lines from `vendor-publish.lock` and changes the import in
`bootstrap/app.go` from `storage/framework/views/vendor/tags` to
`storage/framework/views/modules/tags`.

Framework `v0.46.4` and Hesape `v0.37.0` refuse a publication that carries the
reserved name, so this cannot come back quietly.

### The entities are concrete types over the non-generic model

Hesape `v0.47.0` removed the generic model layer, and this release requires
Hesape `v0.48.0` and Framework `v0.50.2`. Go selects one Hesape version for the
whole build, so an application takes this release after its own models have
moved to the concrete model; `go run github.com/arandu-io/aru/cmd/model-upgrade@latest ./...`
does the mechanical part of that.

An application that uses this package through `New`, its routes and
`TagService` changes nothing: every service method keeps its signature. What
breaks is code that reached the tables directly through `Tags(db)` or
`Taggables(db)`:

| before | now |
|---|---|
| `func Tags(*data.DB) *model.Model[Tag]` | `func Tags(model.DB) *TagQuery`; a `*data.DB` is a `model.DB`, so the call itself compiles unchanged |
| `func Taggables(*data.DB) *model.Model[Taggable]` | `func Taggables(model.DB) *TaggableQuery` |
| `tags.Tags(db).NewQuery().Where(…)` | `tags.Tags(db).Where(…)` |
| `tags.Tags(db).NewInstance(nil, false)`, then `.Entity` | `tags.Tags(db).New()`, which returns the `*Tag` |
| `func(*model.Builder[tags.Tag])` in a grouped where | `func(*tags.TagQuery)` |
| `q.GetQuery()` on a query | `q.Base().GetQuery()` |
| `record.Exists`, a field | `record.Exists()` |
| the fields and methods the generic model promoted on `Tag` and `Taggable` -- `KeyType`, `Incrementing`, `TenantColumn`, `Entity`, `NewQuery`, `Query`, `Where`, `First`, and the rest | gone; a row keeps `Save`, `Delete`, `Fresh`, `Replicate`, `Exists`, `Table` and the attribute methods of `model.Model` |

`Get` on the query returns a `TagCollection`, which is a `[]*Tag`.

**What changes without a compiler error.** A value copy of a `Tag` or a
`Taggable` cannot be saved: the write is refused with `model.ErrUnwired`, where
before it acted on the row the copy was taken from.

## v0.3.1

No package API, route or migration changes. Update the module normally to select Framework v0.47.1, Hesape v0.41.1 and Kyse v0.29.1. Existing authorization and tenant policies are unchanged. Application-owned published views are not overwritten by this dependency update.

## v0.2.3

Nothing to change in an application. This release corrects what the previous
ones said about themselves: `CHANGELOG.md` and this file described the releases
of the package skeleton this repository was cloned from, so `v0.2.2` shipped a
changelog whose highest heading was the skeleton's, with everything this package
added filed as unreleased and its own `v0.2.1` and `v0.2.2` recorded nowhere.

Three tests hold it now: an action declared in `policy.go` and a migration
declared in `module.go` have to be named under a version heading rather than an
unreleased one, and the two files have to describe the same set of versions.

## v0.2.2

Reinstall, and rebuild the views. The published `v0.2.1` archive carried what
the view compiler writes beside the sources rather than the sources: the
compiler treats a module with no `resources/views` as a component library and
writes the compiled Go next to the source, so the build the guides ask for put
compiled views into the archive, published over a page the project had already
generated. Run `aru vendor:publish --tag=view --apply` and `aru view:build`
after upgrading.

## v0.2.1

Nothing to change, and everything to reinstall. The published `v0.2.0` archive
was missing its view sources -- `go mod` drops every path with a segment named
`vendor` when it packs a module, so a project that imported the package failed
to build with `pattern resources/views: no matching files found`. The files land
at the same addresses under the same view names; what changed is where the
archive carries them.

## v0.2.0

Everything under this heading is additive except three things, and each of the
three fails loudly rather than quietly.

### `Config.CSRF` is required

```go
// Before.
tags.New(tags.Config{Tenant: cfg.Auth.Tenant}, db, sessions)

// After.
tags.New(tags.Config{Tenant: cfg.Auth.Tenant, CSRF: csrf}, db, sessions)
```

`New` refuses a configuration without it, at the line that wires the module.
Every screen this package now draws writes, and a page rendered without a token
is a page whose buttons the application refuses -- which is a form that does
nothing, discovered by whoever clicks it.

### A browser gets markup where it used to get JSON

The routes answer in two shapes, chosen by `ctx.WantsJSON()` -- the framework's
own question. A client that sends `Accept: application/json` gets exactly the
body it got before, on the same addresses. A browser, or anything sending no
`Accept` header at all, now gets the screen.

If you were calling these routes from a program without that header, add it.
There is no flag and no second address: two spellings of one route disagree.

### `(*TagService).Move` accepts a negative position

It used to refuse one. The order is a total order over signed integers: the
first claim of a taxonomy lands on zero, so `MoveToStart` on the first label has
to land below it. Keeping zero as a floor would mean renumbering the taxonomy to
open room at the front, which is the work sparse positions exist to avoid.

Nothing that worked before stops working. A caller that only ever passed
non-negative positions is unaffected.

### Write the policy in your application

`TagPolicy` still denies everything and is still what a wiring that says nothing
about rules gets. What is new is that you can replace it without forking the
package:

```go
	tags.Config{
		Tenant: cfg.Auth.Tenant,
		CSRF:   csrf,
		Policy: TagRules{},
	}
```

`Config.Policy` is optional and nil is the shipped refusal. There is still one
policy and one place it is consulted.

### The new surface

`AttachTags`, `DetachTags`, `DetachAllTags`, `SyncTags`, `SyncTagsOfTaxonomy`,
`HasTag`, `TagsOfTaxonomy`, `OwnersWithAnyTagOfTaxonomy`, `FindByName`,
`FindManyByName`, `FindInAnyTaxonomy`, `FindOrCreate`, `Search`, `Taxonomies`,
`UnusedTags`, `Reorder`, `MoveUp`, `MoveDown`, `MoveToStart`, `MoveToEnd` and
`SwapOrder` are all new. Nothing was removed and nothing changed signature.

`Commands(Deps)` returns the six commands; append them to the slice your console
kernel dispatches. `(*Module).Labels(locale)` is the catalogue. The screens are
published the way the single one always was, and there are four of them now, so
run `aru vendor:publish --tag=view --apply` and `aru view:build` again after
upgrading -- `Boot` refuses to serve until every one of them is linked, and the
refusal names the ones that are missing.

## v0.1.0

The first release. Nothing to upgrade from.
