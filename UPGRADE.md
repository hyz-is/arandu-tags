# Upgrade Guide

Every heading below is a tag of this repository. Up to `v0.2.2` this file also
carried two sections describing releases of the package skeleton this repository
was cloned from -- `v0.4.0` and `v0.2.0`, with the entity renamed into them,
describing a publishing migration and a Repository removal that both happened
before `v0.1.0` here. They are gone.

## Unreleased

Nothing yet.

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
