package unit_test

import (
	"testing"

	"github.com/arandu-io/hesape/translation"

	tags "github.com/hyz-is/arandu-tags"
)

// The label of a tag, in a locale.
//
// A row holds one name and no map of language to name. The catalogue that the
// rest of the product reads its sentences from reads this one too, and a tag
// nobody has translated shows the words its creator typed.

// catalogue is a translator holding the lines an application would write into
// lang/<locale>/tags.json.
func catalogue(t *testing.T, lines map[string]string) *translation.Translator {
	t.Helper()

	tr := translation.New(translation.NewArrayLoader(), "pt-BR", "en")
	tr.AddLines(lines, "pt-BR", "")
	return tr
}

func TestALabelTheProductShipsComesFromTheCatalogue(t *testing.T) {
	t.Parallel()

	tr := catalogue(t, map[string]string{
		"tags.status/draft": "Rascunho",
		"tags.published":    "Publicado",
	})

	inTaxonomy := tags.Tag{Type: "status", Name: "Draft", Slug: "draft"}
	if got := tags.Label(tr, "pt-BR", inTaxonomy); got != "Rascunho" {
		t.Fatalf("the label is %q, want Rascunho", got)
	}

	// A tag with no taxonomy is keyed by its slug alone, so the two forms do
	// not have to be spelled the same way in the file.
	untyped := tags.Tag{Type: tags.DefaultTaxonomy, Name: "Published", Slug: "published"}
	if got := tags.Label(tr, "pt-BR", untyped); got != "Publicado" {
		t.Fatalf("the label is %q, want Publicado", got)
	}
}

func TestALabelACustomerInventedShowsWhatTheyTyped(t *testing.T) {
	t.Parallel()

	tr := catalogue(t, map[string]string{"tags.status/draft": "Rascunho"})

	// Nobody has translated this one, and nobody was going to: it is a word a
	// customer chose in a product that has never seen it.
	invented := tags.Tag{Type: "status", Name: "Aguardando jurídico", Slug: "aguardando-juridico"}
	if got := tags.Label(tr, "pt-BR", invented); got != "Aguardando jurídico" {
		t.Fatalf("the label is %q, want what the customer typed", got)
	}

	// A locale the catalogue says nothing about falls through the same way,
	// rather than answering with the key.
	shipped := tags.Tag{Type: "status", Name: "Draft", Slug: "draft"}
	if got := tags.Label(tr, "ja", shipped); got != "Rascunho" && got != "Draft" {
		t.Fatalf("an untranslated locale answered %q", got)
	}
}

func TestALabelWithNoTranslatorIsTheNameOnTheRow(t *testing.T) {
	t.Parallel()

	record := tags.Tag{Type: "status", Name: "Draft", Slug: "draft"}
	if got := tags.Label(nil, "pt-BR", record); got != "Draft" {
		t.Fatalf("the label is %q, want Draft", got)
	}
}

func TestTwoTaxonomiesDoNotShareALabelKey(t *testing.T) {
	t.Parallel()

	// The same slug in two taxonomies is two labels, and each has to be
	// translatable on its own. A key built from the slug alone would give one
	// sentence to both.
	language := tags.Tag{Type: "language", Slug: "go"}
	game := tags.Tag{Type: "board-game", Slug: "go"}
	untyped := tags.Tag{Type: tags.DefaultTaxonomy, Slug: "go"}

	keys := map[string]string{
		"language":   tags.LabelKey(language),
		"board-game": tags.LabelKey(game),
		"untyped":    tags.LabelKey(untyped),
	}
	seen := map[string]string{}
	for taxonomy, key := range keys {
		if previous, taken := seen[key]; taken {
			t.Fatalf("%s and %s share the key %q", previous, taxonomy, key)
		}
		seen[key] = taxonomy
	}

	// And the group is the one an application writes a file for.
	if got := tags.LabelKey(untyped); got != tags.LabelGroup+".go" {
		t.Fatalf("the key of an untyped tag is %q", got)
	}
}

func TestASlugIsWhatDistinguishesTwoNames(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]string{
		"Draft":                  "draft",
		"  Draft  ":              "draft",
		"DRAFT!":                 "draft",
		"Waiting for legal":      "waiting-for-legal",
		"Waiting  --  for legal": "waiting-for-legal",
		"Aguardando jurídico":    "aguardando-jurídico",
		"日本語":                    "日本語",
		"C++":                    "c",
		"":                       "",
		"!!!":                    "",
	} {
		if got := tags.Slugify(name); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", name, got, want)
		}
	}
}
