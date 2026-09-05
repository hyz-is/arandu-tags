# Upgrade Guide

## Unreleased

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

## v0.4.0

Version 0.4.0 hands publishing to the framework. The package no longer defines
the contract or carries the command that writes the files. Upgrade Framework to
`v0.46.0` and Hesape to `v0.25.0` before changing anything below.

### Publish with the CLI

```sh
# Before.
go run github.com/hyz-is/arandu-tags/publish@latest
go run github.com/hyz-is/arandu-tags/publish@latest --force

# After.
aru vendor:publish --tag=view
aru vendor:publish --tag=view --apply
aru vendor:publish --tag=view --apply --force
```

The `publish` command of this module was removed. `aru vendor:publish` asks the
application which modules it registered and writes what each of them declares,
so one command publishes every installed package instead of one command per
package. Without `--apply` it writes nothing and prints what each file would
become; running it twice changes nothing the second time.

`PublishCommand` changed from `go run <module>/publish@latest` to
`aru vendor:publish --apply`. It is what `(*Module).Boot` names in its refusal,
and an application that prints it anywhere of its own gets the new spelling by
recompiling.

### Answer the framework's publishing contract

`Publishable`, declared by this package, was removed. The contract is
`foundation.Publishable` from `github.com/arandu-io/framework/foundation`, and
what it asks for is a list rather than a tree:

```go
// Before.
type Publishable interface {
	Name() string
	Publishes() fs.FS
}

// After.
type Publishable interface {
	Publishes() []foundation.Publication
}
```

`Module.Publishes` changed from `func() io/fs.FS` to
`func() []foundation.Publication`. A `Publication` carries the tag — one of
`view`, `component`, `config`, `migration`, `translation`, `asset` — the tree,
and optionally the directory to read it from and the directory it lands in. This
package declares one, tagged `foundation.PublishView`, with neither directory
set, because every path in its archive is already the path the file takes in the
project.

The package-level `Publishes` function was removed with the command that needed
it: it existed because a `package main` with no database handle could never hold
a `Module`, and there is no such command any more. Reach the declaration through
the module.

### Contracts that did not move

`PublishedPaths`, `ViewNames` and `ViewPackages` are unchanged, and so are the
paths the views land under. A project that already published them is holding the
same files at the same addresses; `aru vendor:publish` reports them as
unchanged rather than rewriting them.

## v0.2.0

Version 0.2.0 replaces the generic CRUD Repository with the configured
Model-first data path. Upgrade Framework to `v0.41.0` and Hesape to `v0.19.1`
before changing the package wiring.

### Replace Repository wiring

Construct the Service with the application database handle:

```go
// Before.
repository := NewTagRepository(db)
service := NewTagService(repository)

// After.
service := NewTagService(db)
```

`TagRepository` and `NewTagRepository` were removed. The removed
generic CRUD methods are `(*TagRepository).Create`,
`(*TagRepository).Delete`, `(*TagRepository).Find`,
`(*TagRepository).List`, and `(*TagRepository).Update`. Use
`Tags(db)` after authorization for generic CRUD. Add a Repository only for
a specialized query, report, projection, read model, export, or external
storage boundary.

### Keep Model results as pointers

The Service now returns the entities owned by the configured Model:

- `(*TagService).Create` changed from `(Tag, error)` to
  `(*Tag, error)`;
- `(*TagService).Find` changed from `(Tag, error)` to
  `(*Tag, error)`;
- `(*TagService).List` changed from `([]Tag, error)` to
  `([]*Tag, error)`;
- `NewTagService` changed from accepting `*TagRepository` to
  accepting `*data.DB`.

Keep those pointers intact until converting them to `Resource` or `Collection`.
Copying an entity with an embedded Model can leave its internal entity pointer
attached to the original allocation.

`Tag`: old is comparable; new is not because it embeds
`model.Model[Tag]`. Do not use the entity as a map key or compare it with
`==`; compare stable fields such as `ID` instead.

### Contracts that did not move

`ErrNotFound`, route names, migration identity, `DefaultPrefix`, and
`DefaultPageSize` remain unchanged. Existing URLs and applied migrations do not
need translation.
