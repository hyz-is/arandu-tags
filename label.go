// The label of a tag, in a locale.
//
// A tag row holds one name: the words whoever created it typed. It holds no
// second name in a second language, and no map of language to name, and that
// absence is the decision this file exists to record.
//
// A stack that resolves a key into a sentence in a locale already exists, it is
// a catalogue of files the application owns, and it answers every sentence the
// product says. Giving a tag a column of translations would put a second
// answer to the same question beside it -- one written by the product in its
// language files, one written per row -- and the two would be maintained by
// different people, in different places, with nothing to say when they
// disagreed.
//
// So the catalogue answers this one too. A tag whose label the product ships
// -- a status, a stage, a severity, anything an application seeds and then
// translates -- gets its sentence from the same lang directory as every other
// sentence, under a key derived from the tag. A tag a customer invented has no
// entry there, and the name they typed is shown, which is correct: nobody has
// translated it, and inventing a translation for it is not this package's job.
//
// The result is one place to look, per locale, for every label a product says
// in its own voice, and no column that goes stale in a language nobody on the
// team reads.

package tags

import "github.com/arandu-io/hesape/translation"

// LabelGroup is the catalogue group a tag's label is read from, which is the
// file the application writes: lang/<locale>/tags.json.
const LabelGroup = "tags"

// LabelKey is the catalogue key a tag's label is read under.
//
// The item is the slug, prefixed by the taxonomy when the tag has one:
//
//	tags.draft              a tag with no taxonomy, slug "draft"
//	tags.status/draft       the same slug in the taxonomy "status"
//
// The separator is a character neither a taxonomy nor a slug may hold, so two
// tags produce one key only when they are the same tag, and a taxonomy cannot
// take over the keys of the one with no name.
func LabelKey(record Tag) string {
	if record.Type == DefaultTaxonomy {
		return LabelGroup + "." + record.Slug
	}
	return LabelGroup + "." + record.Type + "/" + record.Slug
}

// Label returns the label to show for a tag in a locale.
//
// It answers with the catalogue line when the application has written one for
// this tag, and with the name stored on the row when it has not. A nil
// translator is the second case: an application that renders no sentences in
// more than one language shows the names its customers typed, which is what it
// would have shown anyway.
//
// It is the whole of what this package does about language. There is nothing
// to write into the row to make a tag translatable, and nothing to migrate when
// a locale is added: the application adds a file.
func Label(tr *translation.Translator, locale string, record Tag) string {
	if tr == nil {
		return record.Name
	}
	key := LabelKey(record)
	if !tr.Has(locale, key) {
		return record.Name
	}
	return tr.Get(locale, key, nil)
}
