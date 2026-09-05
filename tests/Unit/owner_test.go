package unit_test

import (
	"errors"
	"strings"
	"testing"

	tags "github.com/hyz-is/arandu-tags"
)

// The kind on the other side of an association, and why it is a value the
// owning domain declares rather than the Go type of its entity.
//
// A Go type name is not stable against anything that leaves the rows alone: a
// package rename, a move into an internal directory, a vendoring, an alias.
// Every one of those changes what reflection would report while every stored
// association goes on carrying what it reported before, and nothing says so.
// The tests here hold the shape of the value that replaced it.

func TestAnOwnerKindIsStoredAsItsNameAndVersion(t *testing.T) {
	t.Parallel()

	kind := tags.MustOwnerType("article", 1)
	if kind.String() != "article.v1" {
		t.Fatalf("the kind is stored as %q, want article.v1", kind.String())
	}
	if kind.Name() != "article" || kind.Version() != 1 {
		t.Fatalf("the kind reads back as %q v%d", kind.Name(), kind.Version())
	}
}

func TestAVersionMakesADifferentKind(t *testing.T) {
	t.Parallel()

	first := tags.MustOwnerType("article", 1)
	second := tags.MustOwnerType("article", 2)

	if first.String() == second.String() {
		t.Fatal("two versions of a kind are stored under one value, so the associations of the first would answer for the second")
	}
	// The values are comparable, which is what lets an application hold one as
	// a package-level constant and compare it with what came out of a row.
	if first == second {
		t.Fatal("two versions of a kind compare equal")
	}
	if first != tags.MustOwnerType("article", 1) {
		t.Fatal("one kind declared twice does not compare equal to itself")
	}
}

func TestAStoredKindReadsBackAsItself(t *testing.T) {
	t.Parallel()

	for _, kind := range []tags.OwnerType{
		tags.MustOwnerType("article", 1),
		tags.MustOwnerType("blog-post", 12),
		tags.MustOwnerType("a", tags.MaxOwnerVersion),
		tags.MustOwnerType(strings.Repeat("a", tags.MaxOwnerNameLen), 3),
	} {
		back, err := tags.ParseOwnerType(kind.String())
		if err != nil {
			t.Fatalf("reading %q back: %v", kind, err)
		}
		if back != kind {
			t.Fatalf("%q read back as %q", kind, back)
		}
	}
}

func TestAValueThatCannotBeAKindIsRefused(t *testing.T) {
	t.Parallel()

	// Every one of these would otherwise be written into a column and read back
	// as something else, or as nothing.
	for name, stored := range map[string]string{
		"no version":              "article",
		"empty name":              ".v1",
		"version is not a number": "article.vx",
		"version is zero":         "article.v0",
		"version is too high":     "article.v1000",
		"uppercase":               "Article.v1",
		"starts with a digit":     "1article.v1",
		"holds a separator":       "article.thing.v1",
		"holds a slash":           "article/thing.v1",
		"holds a space":           "blog post.v1",
		"empty":                   "",
	} {
		if _, err := tags.ParseOwnerType(stored); !errors.Is(err, tags.ErrOwnerType) {
			t.Errorf("%s (%q) was read as a kind: %v", name, stored, err)
		}
	}

	// And the declaration form refuses the same values, loudly, because a kind
	// written into a source file is a mistake in the program rather than input.
	for _, bad := range []struct {
		name    string
		version int
	}{
		{"", 1},
		{"Article", 1},
		{"article", 0},
		{"article", tags.MaxOwnerVersion + 1},
		{strings.Repeat("a", tags.MaxOwnerNameLen+1), 1},
	} {
		func() {
			defer func() {
				if recovered := recover(); recovered == nil {
					t.Errorf("MustOwnerType(%q, %d) returned a kind", bad.name, bad.version)
				}
			}()
			_ = tags.MustOwnerType(bad.name, bad.version)
		}()
	}
}

func TestTheZeroKindNamesNothing(t *testing.T) {
	t.Parallel()

	var kind tags.OwnerType
	if !kind.IsZero() {
		t.Fatal("the zero kind does not report itself as one")
	}
	if kind.String() != "" {
		t.Fatalf("the zero kind is stored as %q", kind.String())
	}
	if _, err := tags.Ref(kind, "article-1"); !errors.Is(err, tags.ErrOwnerType) {
		t.Fatal("a reference to no kind was built")
	}
}

func TestAReferenceIsCheckedWhereItIsBuilt(t *testing.T) {
	t.Parallel()

	kind := tags.MustOwnerType("article", 1)

	ref, err := tags.Ref(kind, "article-1")
	if err != nil {
		t.Fatalf("building a reference: %v", err)
	}
	if ref.Type() != kind || ref.ID() != "article-1" {
		t.Fatalf("the reference reads back as %q %q", ref.Type(), ref.ID())
	}
	if ref.String() != "article.v1:article-1" {
		t.Fatalf("the reference renders as %q", ref.String())
	}

	if _, err := tags.Ref(kind, ""); !errors.Is(err, tags.ErrOwnerType) {
		t.Fatal("a reference with no identifier was built")
	}
	if _, err := tags.Ref(kind, strings.Repeat("x", tags.MaxOwnerIDLen+1)); !errors.Is(err, tags.ErrOwnerType) {
		t.Fatal("a reference with an identifier past the maximum was built")
	}

	// The zero reference is unusable, which is what the service checks rather
	// than trusting a struct somebody filled in field by field.
	var empty tags.OwnerRef
	if !empty.IsZero() || empty.String() != "" {
		t.Fatal("the zero reference does not report itself as one")
	}
}
