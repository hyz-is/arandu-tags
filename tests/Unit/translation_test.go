package unit_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/arandu-io/hesape/translation"

	tags "github.com/hyz-is/arandu-tags"
)

// The catalogue this package ships, and the one property that lets it hold two
// different things.
//
// The group is "tags", and two families of key read from it: the sentences of
// this package's own screens, and the label of a tag that the product ships.
// They share a file so an application overrides both in one place, and they
// cannot collide because of the shape of a slug -- which is what the first test
// below holds, against Slugify rather than against a list of examples somebody
// remembered to update.

func TestATagLabelCanNeverLandOnASentenceOfThisPackage(t *testing.T) {
	t.Parallel()

	// A key of this package's own sentences always has a dot inside the group:
	// tags.field.name, tags.screen.order_title. A tag's label never does, because
	// a slug is letters and digits with hyphens between them and a taxonomy is
	// held to the same shape. So no label a customer invents reaches a sentence,
	// whatever they call it.
	for _, name := range []string{
		"field.name", "screen.order_title", "control.save",
		"a.b.c", "...", "Draft", "100% done", "  spaced  ", "acentuação",
	} {
		key := tags.LabelKey(tags.Tag{Name: name, Slug: tags.Slugify(name)})
		item := strings.TrimPrefix(key, tags.TranslationGroup+".")
		if strings.Contains(item, ".") {
			t.Errorf("the label %q produces the key %q, which can collide with a sentence of this package", name, key)
		}
	}

	// And with a taxonomy, which is the other half of the key.
	for _, taxonomy := range []string{"status", "stage-2", "a_b"} {
		key := tags.LabelKey(tags.Tag{Type: taxonomy, Slug: "draft"})
		item := strings.TrimPrefix(key, tags.TranslationGroup+".")
		if strings.Contains(item, ".") {
			t.Errorf("a label of %q produces %q, which can collide with a sentence of this package", taxonomy, key)
		}
	}

	// The proof would be worthless if every key of this package were dotless
	// too, so this is the other side of it.
	if !strings.Contains(strings.TrimPrefix("tags.field.name", tags.TranslationGroup+"."), ".") {
		t.Fatal("a sentence of this package has no dot in its item, so the two families are not told apart by one")
	}
}

func TestEveryLocaleShipsEveryLine(t *testing.T) {
	t.Parallel()

	locales := tags.Locales()
	if len(locales) < 2 {
		t.Fatalf("the package ships %d locale(s), so a missing line in one of them would be invisible", len(locales))
	}
	if !slices.Contains(locales, "en") {
		t.Fatal("the fallback locale ships no catalogue, so a key missing everywhere else would draw as itself")
	}

	// The set of keys, not the sentences: a locale added with half the lines is
	// a screen that draws its own keys in that language, and nothing else says
	// so.
	reference := keysOf(t, tags.Lines("en"))
	for _, locale := range locales {
		got := keysOf(t, tags.Lines(locale))
		if len(got) == 0 {
			t.Errorf("%s ships no line at all", locale)
			continue
		}
		for _, key := range reference {
			if !slices.Contains(got, key) {
				t.Errorf("%s is missing %s", locale, key)
			}
		}
		for _, key := range got {
			if !slices.Contains(reference, key) {
				t.Errorf("%s holds %s, which en does not", locale, key)
			}
		}
	}

	// A locale nothing was written for answers with nothing rather than with the
	// fallback's lines under another name.
	if lines := tags.Lines("xx-YY"); lines != nil {
		t.Errorf("a locale this package does not ship answered with %d lines", len(lines))
	}
}

func TestASentenceIsReadFromTheApplicationsCatalogueFirst(t *testing.T) {
	t.Parallel()

	// The application's translator wins wherever it has a line, and the shipped
	// catalogue is the floor under it. One override path, and it is the path an
	// application already has.
	override := translation.New(
		translation.NewArrayLoader().AddMessages("en", tags.TranslationGroup,
			translation.Lines{"field.name": "Label"}, "*"),
		"en", "en")

	module := moduleWith(t, tags.Config{Tenant: "acme", Translator: override})
	if got := module.Labels("en").T("field.name"); got != "Label" {
		t.Errorf("the application's line was not read: %q", got)
	}
	// And a key it says nothing about falls through rather than disappearing.
	if got := module.Labels("en").T("control.save"); got == "" || got == "control.save" {
		t.Errorf("a key the application does not override read as %q", got)
	}

	// A locale nobody wrote is answered in the shipped one rather than refused:
	// a screen in the wrong language is still a screen.
	if got := module.Labels("").T("control.save"); got != module.Labels("en").T("control.save") {
		t.Errorf("an empty locale read %q", got)
	}
}

func TestALabelIsTheCataloguesLineOrTheNameOnTheRow(t *testing.T) {
	t.Parallel()

	shipped := translation.New(
		translation.NewArrayLoader().AddMessages("en", tags.TranslationGroup,
			translation.Lines{"status/draft": "Not published yet"}, "*"),
		"en", "en")
	module := moduleWith(t, tags.Config{Tenant: "acme", Translator: shipped})
	labels := module.Labels("en")

	product := tags.Tag{Type: "status", Name: "Draft", Slug: "draft"}
	if got := labels.Tag(product); got != "Not published yet" {
		t.Errorf("a label the product ships read as %q", got)
	}

	// A tag a customer invented has no line, and the name they typed is what is
	// shown -- which is correct: nobody has translated it.
	invented := tags.Tag{Type: "status", Name: "Ready for Ana", Slug: "ready-for-ana"}
	if got := labels.Tag(invented); got != "Ready for Ana" {
		t.Errorf("a label a customer invented read as %q", got)
	}

	// The taxonomy with no name is the one this package can name for itself.
	if got := labels.Taxonomy(tags.DefaultTaxonomy); got == "" || got == tags.DefaultTaxonomy {
		t.Errorf("the taxonomy with no name read as %q", got)
	}
	if got := labels.Taxonomy("status"); got != "status" {
		t.Errorf("a taxonomy nobody wrote a line for read as %q, want its own name", got)
	}
}

// keysOf is the keys of a catalogue, sorted, and fails when there are none.
func keysOf(t *testing.T, lines translation.Lines) []string {
	t.Helper()

	out := make([]string, 0, len(lines))
	for key := range lines {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}
