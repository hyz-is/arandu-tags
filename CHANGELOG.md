# Changelog

Everything worth knowing about a release of Arandu Tags is recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
the versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

A published module version is immutable: Go serves it from the proxy forever, so
a release is corrected by another release and never by moving a tag.

Every heading below is a tag of this repository. Up to `v0.2.2` this file also
carried three sections describing releases of the package skeleton this
repository was cloned from -- numbered `0.2.0`, `0.3.1` and `0.4.0`, dated before
this repository existed, and numbered over the tags of the same name here. They
are gone.

## [Unreleased]

## [0.2.3] - 2026-09-06

### Added

- Three tests that hold the two release files against the code: an action
  declared in `policy.go` and a migration declared in `module.go` have to be
  named under a version heading rather than under `[Unreleased]`, and the two
  files have to describe the same set of versions.

### Fixed

- This file and `UPGRADE.md` describe the releases of this package. Both
  carried the package skeleton's own release history -- `0.4.0`, `0.3.1` and
  `0.2.0`, dated before this repository existed and numbered over the tags of
  the same name here -- because `configure` renames the template's values into
  those files and never resets them. So `v0.2.2` shipped a changelog whose
  highest heading described a publishing migration of the skeleton, with
  everything this package actually added filed under `[Unreleased]` and its own
  `v0.2.1` and `v0.2.2` recorded nowhere.

## [0.2.2] - 2026-09-05

### Fixed

- The archive carries the view sources rather than what the compiler writes
  beside them. The view compiler decides by the existence of
  `resources/views`: without it, it treats the module as a component library
  and writes the compiled Go next to the source, so running the build the
  guides ask for put compiled views into the archive -- published over a page
  the project had already generated. The embed pattern names the extension now,
  and the output is ignored.
- The published view prefix is trimmed once. The second trim was against the
  empty string, which removes nothing.

## [0.2.1] - 2026-09-05

### Fixed

- The published module carries its view sources. They were kept at
  `resources/views/vendor/tags/`, which is the address an application looks for
  them at, and `go mod` drops every path with a segment named `vendor` when it
  packs a module, at any depth -- so the files were in the repository and absent
  from what anybody downloads, and an import failed on the embed with
  `pattern resources/views: no matching files found` on a tree where every gate
  here was green. The archive keeps them under `resources/publish/` and the
  publication carries where they come from and where they go, so they still land
  at the same addresses.

### Added

- Two gates beside the fix: one refuses a tracked resource path with a `vendor`
  segment, and the other refuses a published view no handler renders, with
  `Fragments` naming the one the application draws itself.

## [0.2.0] - 2026-09-05

### Added

- Finding a label by what somebody typed: `(*TagService).FindByName`,
  `FindManyByName`, `FindInAnyTaxonomy`, `FindOrCreate`, `Search` and
  `Taxonomies`, with `(Tag).Matches` and the bounds `MinSearchLen` and
  `MaxSearchLen`. A name and the slug it reduces to are both accepted, because
  both are spellings of one label.
- Set operations over what an entity carries: `(*TagService).AttachTags`,
  `DetachTags`, `DetachAllTags`, `SyncTags`, `SyncTagsOfTaxonomy`, `HasTag`,
  `TagsOfTaxonomy` and `OwnersWithAnyTagOfTaxonomy`. They answer "make this the
  set, and say what changed", which is a different question from the one
  `Attach` and `Detach` answer -- so the same request sent twice through them
  leaves the same set instead of being refused.
- Ordering: `(*TagService).Reorder`, `MoveUp`, `MoveDown`, `MoveToStart`,
  `MoveToEnd` and `SwapOrder`, with `ErrOrderIncomplete`, `ErrOrderChanged` and
  `ErrDifferentTaxonomies`. A swap goes through a position claimed from the
  counter, so no row passes through a place another row holds.
- `(*TagService).UnusedTags`, the labels of a taxonomy that nothing carries.
- Six commands, built by `Commands(Deps)` and registered by the application:
  `tags:list`, `tags:create`, `tags:rename`, `tags:reorder`, `tags:taxonomies`
  and `tags:prune`. `CommandPrefix` is the namespace they share.
- A catalogue of sentences, embedded and never published, in `en` and `pt-BR`:
  `TranslationGroup`, `FallbackLocale`, `Locales`, `Lines`, `Labels`,
  `(Labels).T`, `(Labels).Tag`, `(Labels).Taxonomy` and `(*Module).Labels`.
- Four screens instead of one: `ViewIndex`, `ViewEdit`, `ViewOrder` and
  `ViewPicker`, with `Row`, `IndexPageData`, `EditPageData`, `OrderPageData`,
  `PickerData`, `FormState`, `(*Module).Rows` and `(*Module).PickerRows`.
  `ViewPicker` is a fragment an application draws inside a page of its own,
  because attaching a label to an entity is a question about that entity.
- Five routes beside the three that were there: `tags.update`, `tags.destroy`,
  `tags.order`, `tags.reorder` and `tags.move`.
- `Config.CSRF`, `Config.Policy` and `Config.Translator`.
- `example/`, behind the `example` build tag, which wires the package and walks
  the whole surface against SQLite in a temporary directory.

### Changed

- `Config.CSRF` is **required**. Every screen this module draws writes, and a
  page rendered without a token is a page whose buttons the application refuses.
- The rules can be written by the application, through `Config.Policy`. Nil is
  still `TagPolicy`, which still denies everything: what changed is that an
  application which installed this package with `go get` can now open an action
  without forking it.
- Every handler answers a screen or a resource, chosen by `ctx.WantsJSON()`. A
  browser gets markup where it used to get JSON; a client sending
  `Accept: application/json` gets exactly what it got before.
- `(*TagService).Move` accepts a negative position. The order is a total order
  over signed integers, and the first claim of a taxonomy lands on zero, so a
  label moved before it has to land below zero -- the alternative is renumbering
  the taxonomy to open room at the front, which is the work sparse positions
  exist to avoid.

## [0.1.0] - 2026-09-05

### Added

- `Tag`, `Taggable` and the tables `tags`, `taggables` and `tag_sequences`,
  created by `20260905_0001_create_tag_tables`.
- `TagPolicy`, and the actions it answers about: `TagView`, `TagList`,
  `TagCreate`, `TagUpdate` and `TagDelete`.
- `OwnerType` and `OwnerRef`, with `MustOwnerType`, `ParseOwnerType` and `Ref`.
  The kind of entity on the other side of an association is declared by the
  domain that owns it and never derived from a Go type.
- `(*TagService).Attach`, `Detach`, `TagsOf`, `OwnersWithAnyTag`,
  `OwnersWithAllTags`, `OwnersWithoutAnyTag`, `Rename` and `Move`.
- `Label` and `LabelKey`, which read a tag's label out of the application's
  translation catalogue and fall back to the name on the row.
- `Slugify`, `ValidTaxonomy`, `DefaultTaxonomy` and `PositionStep`.
- `TagAttach` and `TagDetach`, the two actions an association is decided by.
- `(*Module).Service`, for an application that attaches from its own handler.
- `ErrSlugTaken`, `ErrPositionUnavailable`, `ErrAlreadyAttached` and
  `ErrNotAttached`.
- `(*TagService).List` takes the taxonomy to list, before the query, and
  `CreateRequest` carries one. A `Tag` carries `Type`, `Slug` and `Position`
  beside `Name`.
