# Contributing

Thank you for considering a contribution.

## Before you open a pull request

Run the four gates. They are what CI runs, and a pull request that fails one of
them fails for everybody who pulls after it.

```bash
gofmt -l $(find . -name '*.go' -not -path '*/testdata/*' -not -name '*.kyse.go')
go build ./...
go vet ./...
go test -race ./...
```

The two filters on `gofmt` are not preference. `gofmt` is the only tool in the
chain that ignores build tags, so it reads files the compiler deliberately
excludes; `testdata/` holds fixtures that are invalid on purpose.

## What a change has to keep true

Four properties are the reason this package is what it is. A change that breaks
one of them will not be merged, whatever else it improves.

1. **The policy denies by default.** No branch allows every action, and none is
   added. Opening an action means writing the rule that opens it.
2. **Every Service method authorizes before reaching the Model.** The Grant is
   then required by the Model terminal; constructing `Tags(db)` first is
   already the wrong order and the structural audit rejects it.
3. **The tenant comes from `data.Tenant(g)`.** Not from the path, the body, the
   query string or a header, on any code path, including the read paths.
4. **`arandu.mod.toml` matches the code.** Adding an outbound call, a file
   write or a process means declaring it there in the same commit.

CRUD stays on `Tags(db)` and the query it returns. Add a Repository only for a
specialized complex query, read model, report, export or raw SQL contract; a
wrapper around `Find`, `Get`, `Save` or `Delete` is a second data path.

Model-backed entities stay pointers. A value copy of `Tag` holds the embedded
Model of the row it was copied from, and a write through it is refused with
`model.ErrUnwired`. Response resources are the deliberate snapshot boundary;
Service results are not.

`TagQuery.go`, `TaggableQuery.go` and `internal/sequence/CounterQuery.go` are
written by `aru model:build` and are not edited by hand. Run it after changing
an entity or its table; `aru model:build --check` fails while they are stale.

## Style

Everything in the source is in English: identifiers, doc comments, internal
comments, error messages, log messages, and the names and messages of tests.
`pkg.go.dev` publishes the doc comments, and its readers are users of this
package.

Every exported symbol carries a doc comment saying what it does. The comment
documents the symbol and nothing else: no dates, no issue numbers, no
references to other repositories. Why a signature is what it is belongs there
when it is a fact about the code; anything else belongs in the pull request.

## Commits

One commit per logical change. Formatting is not mixed with behaviour, because
a diff that does both is a diff nobody reviews.

## Reporting a vulnerability

Do not open an issue. See [SECURITY.md](SECURITY.md).

## Licence

By contributing you agree that your contribution is licensed under the MIT
licence, the same as the rest of github.com/hyz-is/arandu-tags.
