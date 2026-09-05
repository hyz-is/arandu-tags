# Changelog

Everything worth knowing about a release of Arandu Tags is recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
the versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

A published module version is immutable: Go serves it from the proxy forever, so
a release is corrected by another release and never by moving a tag.

## [Unreleased]

The entries below 0.4.0 are the template this package was cloned from. Nothing
under this heading has been released yet.

### Added

- `Tag`, `Taggable` and the tables `tags`, `taggables` and `tag_sequences`.
- `OwnerType` and `OwnerRef`, with `MustOwnerType`, `ParseOwnerType` and `Ref`.
  The kind of entity on the other side of an association is declared by the
  domain that owns it and never derived from a Go type.
- `(*TagService).Attach`, `Detach`, `TagsOf`, `OwnersWithAnyTag`,
  `OwnersWithAllTags`, `OwnersWithoutAnyTag`, `Rename` and `Move`.
- `Label` and `LabelKey`, which read a tag's label out of the application's
  translation catalogue and fall back to the name on the row.
- `Slugify`, `ValidTaxonomy`, `DefaultTaxonomy` and `PositionStep`.
- `TagAttach` and `TagDetach` beside the five actions the template shipped.
- `(*Module).Service`, for an application that attaches from its own handler.
- `ErrSlugTaken`, `ErrPositionUnavailable`, `ErrAlreadyAttached` and
  `ErrNotAttached`.
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

- `(*TagService).List` takes the taxonomy to list, before the query.
- `CreateRequest` carries `Taxonomy`.
- `Tag` carries `Type`, `Slug` and `Position` beside `Name`.
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

## [0.4.0] - 2026-09-05

### Added

- `(*Module).Publishes` declares one `foundation.Publication`, tagged as a view.
  The contract belongs to the framework, so whatever writes the files reads
  every module through one interface instead of one this package defined for
  itself.

### Changed

- The minimum Framework version is now `v0.46.0`, with Hesape `v0.25.0`.
- `(*Module).Publishes` returns `[]foundation.Publication` instead of `io/fs.FS`.
- `PublishCommand` is now `aru vendor:publish --apply`.
- `(*Module).Boot` names the package whose import links the views, alongside the
  view and the command.

### Removed

- `Publishable`, the contract this package declared for itself.
  `foundation.Publishable` is the one it answers now.
- `Publishes`, the package-level function. There was a second form because a
  command with no database handle could not hold a `Module`; there is no such
  command any more.
- `publish`, the command of this module. `aru vendor:publish` reads the modules
  an application registered and writes what each one declares, which is a
  question only the application can answer.

## [0.3.1] - 2026-09-03

### Added

- `Publishable`, the optional contract a module answers to hand files to the
  application, and `Publishes()` on `Module`.
- `PublishedPaths`, `ViewNames` and `ViewPackages`, the three spellings of one
  view derived from the archive rather than written down separately.
- `PublishCommand`, the one spelling of the command that copies the views.
- `publish`, a command of this module: `go run <module>/publish@latest` writes
  the views under `resources/views/vendor/<module>/`, refuses to replace a file
  the project already has without `--force`, and prints the imports that link
  them.
- `(*Module).Boot` refuses to serve when a view this package renders was never
  published, naming the view and the command instead of answering the first
  request that reaches it with a 500.

## [0.2.0] - 2026-08-29

### Added

- `Tags(db)` exposes the configured, tenant-scoped Model used by the
  Service after authorization.

### Changed

- The minimum Framework version is now `v0.41.0`, with Hesape `v0.19.1`.
- `NewTagService` now accepts `*data.DB` instead of
  `*TagRepository`.
- `(*TagService).Create` now returns `(*Tag, error)`.
- `(*TagService).Find` now returns `(*Tag, error)`.
- `(*TagService).List` now returns `([]*Tag, error)`.
- `Tag`: old is comparable; new is not because it embeds
  `model.Model[Tag]`. Compare stable fields such as `ID` instead.

### Removed

- `TagRepository` and `NewTagRepository`.
- `(*TagRepository).Create`, `(*TagRepository).Delete`,
  `(*TagRepository).Find`, `(*TagRepository).List`, and
  `(*TagRepository).Update`. Add a Repository only for specialized
  queries, reports, projections, read models, exports, or external storage.
