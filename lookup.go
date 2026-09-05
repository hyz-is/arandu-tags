// Finding a label by what somebody typed, and creating the ones that are not
// there yet.
//
// A label is addressed by its identifier everywhere a machine talks to this
// package. A person types a name, and a name is not an identifier: two
// taxonomies hold the same one, whoever typed it may have typed the slug
// instead, and the label may not exist at all. Everything in this file exists to
// turn what was typed into rows, and to say which of the three happened.

package tags

import (
	"context"
	"fmt"
	"strings"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/database/model"
)

// The bounds a search is held to.
const (
	// MinSearchLen is the shortest term a search accepts. One character matches
	// most of a taxonomy, which is the same answer as no filter and costs a scan
	// to produce.
	MinSearchLen = 2
	// MaxSearchLen is the longest term a search accepts.
	MaxSearchLen = MaxNameLen
	// likeEscape is the character that makes a wildcard literal inside a search
	// term. It is not the backslash because a backslash is itself an escape
	// inside a string literal on one of the engines this runs on, and the clause
	// would stop parsing there.
	likeEscape = '!'
)

// Matches reports whether this label is the one somebody typed.
//
// The name as it was written, or the slug it reduces to: those are the two
// spellings of one label a person can be holding, and a lookup that accepted
// only the first would miss every label copied out of a URL.
func (t Tag) Matches(value string) bool {
	return t.Name == value || (t.Slug != "" && t.Slug == Slugify(value))
}

// FindByName returns the label of this taxonomy that somebody typed, and
// ErrNotFound when the taxonomy holds none.
//
// A name and a slug are both accepted, because both are spellings of one label
// and a caller holding one of them cannot always tell which it has.
func (s *TagService) FindByName(ctx context.Context, actor security.Subject, taxonomy, value string) (*Tag, error) {
	found, err := s.FindManyByName(ctx, actor, taxonomy, []string{value})
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, ErrNotFound
	}
	return found[0], nil
}

// FindManyByName returns the labels of this taxonomy that these names or slugs
// name, in one statement.
//
// One statement and not one per name: a caller resolving the ten labels of a
// form is asking one question, and answering it ten times is ten round trips
// whose cost the caller never chose. Names that name nothing are simply absent
// from the answer, which is what makes the result usable as the set that exists.
func (s *TagService) FindManyByName(ctx context.Context, actor security.Subject, taxonomy string, values []string) ([]*Tag, error) {
	g, err := security.Authorize(ctx, s.policy, actor, TagList, Tag{})
	if err != nil {
		return nil, err
	}
	if !ValidTaxonomy(taxonomy) {
		return nil, fmt.Errorf("tags: %q cannot be a taxonomy", taxonomy)
	}
	names, slugs, err := lookupTerms(values)
	if err != nil || len(names) == 0 {
		return nil, err
	}

	return Tags(s.db).NewQuery().
		Where("type", "=", taxonomy).
		Where(func(named *model.Builder[Tag]) {
			named.WhereIn("name", anys(names)).
				OrWhere(func(slugged *model.Builder[Tag]) {
					slugged.WhereIn("slug", anys(slugs))
				})
		}).
		OrderBy("position").OrderBy("id").
		Limit(maxLimit).
		Get(ctx, g)
}

// FindInAnyTaxonomy returns every label that this name or slug names, whichever
// taxonomy each of them belongs to.
//
// It answers with all of them rather than with one, and that is the whole reason
// it is separate from FindByName: one name identifies one label inside a
// taxonomy and identifies nothing across two. A caller that has only a word --
// out of a search box, out of an import file -- gets the labels it could mean,
// and picks.
func (s *TagService) FindInAnyTaxonomy(ctx context.Context, actor security.Subject, value string) ([]*Tag, error) {
	g, err := security.Authorize(ctx, s.policy, actor, TagList, Tag{})
	if err != nil {
		return nil, err
	}
	names, slugs, err := lookupTerms([]string{value})
	if err != nil || len(names) == 0 {
		return nil, err
	}

	return Tags(s.db).NewQuery().
		Where(func(named *model.Builder[Tag]) {
			named.WhereIn("name", anys(names)).
				OrWhere(func(slugged *model.Builder[Tag]) {
					slugged.WhereIn("slug", anys(slugs))
				})
		}).
		OrderBy("type").OrderBy("position").OrderBy("id").
		Limit(maxLimit).
		Get(ctx, g)
}

// FindOrCreate returns the labels these names name in this taxonomy, creating
// the ones that are not there.
//
// The answer has one label per name given, in the order the names were given, so
// a caller can pair them up without matching strings a second time. Two names
// that reduce to one slug are one label and are answered with the same row: they
// are the same label, and creating a second one would be creating a label
// nothing can tell from the first.
//
// The labels that already exist are read in one statement, and only what is
// missing is written -- so the common case, where a form resubmits the labels it
// was rendered with, writes nothing at all.
func (s *TagService) FindOrCreate(ctx context.Context, actor security.Subject, taxonomy string, names []string) ([]*Tag, error) {
	existing, err := s.FindManyByName(ctx, actor, taxonomy, names)
	if err != nil {
		return nil, err
	}

	bySlug := make(map[string]*Tag, len(existing))
	for _, record := range existing {
		bySlug[record.Slug] = record
	}

	out := make([]*Tag, 0, len(names))
	for _, name := range names {
		slug := Slugify(name)
		record, held := bySlug[slug]
		if !held {
			record, err = s.Create(ctx, actor, CreateRequest{Name: name, Taxonomy: taxonomy})
			if err != nil {
				return nil, err
			}
			// Written back, so a name repeated later in the same call is
			// answered with the row this iteration created rather than creating
			// a second one the unique index would refuse.
			bySlug[slug] = record
		}
		out = append(out, record)
	}
	return out, nil
}

// Search returns a page of the labels of one taxonomy whose name contains this
// term, whatever case either is written in.
//
// It matches the name and not the slug. The slug is what a machine addresses a
// label by; the name is what a person reads, and a person typing into a search
// box is looking for what they read.
//
// The term is matched literally: the characters a LIKE clause would read as
// wildcards are escaped, so somebody searching for a per-cent sign finds labels
// with one instead of every label there is.
func (s *TagService) Search(ctx context.Context, actor security.Subject, taxonomy, term string, q data.Query) ([]*Tag, error) {
	g, err := security.Authorize(ctx, s.policy, actor, TagList, Tag{})
	if err != nil {
		return nil, err
	}
	if !ValidTaxonomy(taxonomy) {
		return nil, fmt.Errorf("tags: %q cannot be a taxonomy", taxonomy)
	}
	term = strings.TrimSpace(term)
	switch {
	case len(term) < MinSearchLen:
		return nil, fmt.Errorf("tags: a search term is at least %d characters", MinSearchLen)
	case len(term) > MaxSearchLen:
		return nil, fmt.Errorf("tags: a search term is at most %d characters", MaxSearchLen)
	}

	column, ok := sortableTag[q.Sort]
	if !ok {
		return nil, fmt.Errorf("tags: sort field not allowed: %q", q.Sort)
	}
	limit := q.Limit
	switch {
	case limit <= 0:
		limit = defaultLimit
	case limit > maxLimit:
		limit = maxLimit
	}

	page := Tags(s.db).NewQuery().Where("type", "=", taxonomy)
	// The column is a constant of this package and the term is a binding, so
	// nothing a caller wrote reaches the statement as text. Lowering both sides
	// is what makes the answer the same on an engine whose LIKE ignores case and
	// on one whose LIKE does not.
	page.GetQuery().WhereRaw("lower(name) like ? escape '"+string(likeEscape)+"'", "%"+likeTerm(term)+"%")
	return page.OrderBy(column).OrderBy("id").Limit(limit).Get(ctx, g)
}

// Taxonomies are the taxonomies this customer holds a label in, sorted.
//
// It reads the labels rather than a list somebody maintains, because there is no
// such list: a taxonomy exists exactly while a label is in it, and one that has
// been emptied has stopped existing. That is what makes the answer usable as the
// tabs of a screen.
func (s *TagService) Taxonomies(ctx context.Context, actor security.Subject) ([]string, error) {
	g, err := security.Authorize(ctx, s.policy, actor, TagList, Tag{})
	if err != nil {
		return nil, err
	}

	found, err := Tags(s.db).NewQuery().
		GroupBy("type").
		OrderBy("type").
		Limit(MaxIDsPerQuery).
		Pluck(ctx, g, "type")
	if err != nil {
		return nil, err
	}
	// The empty string is DefaultTaxonomy and is a taxonomy like any other, so
	// it is kept rather than filtered out: a screen that dropped it would hide
	// every label created without one.
	return distinct(texts(found)), nil
}

// lookupTerms turns what a caller typed into the two lists a lookup matches on:
// the names as written, and the slugs they reduce to.
//
// A value that reduces to no slug is refused rather than ignored. It cannot name
// a label -- nothing this package writes has an empty slug -- so a lookup that
// accepted it would answer "not found" for an input that was never a question.
func lookupTerms(values []string) (names, slugs []string, err error) {
	if len(values) == 0 {
		return nil, nil, nil
	}
	if len(values) > MaxIDsPerQuery {
		return nil, nil, fmt.Errorf("tags: %d names were given, and at most %d may be", len(values), MaxIDsPerQuery)
	}
	for _, value := range values {
		if len(value) > MaxNameLen {
			return nil, nil, fmt.Errorf("tags: a name is at most %d characters", MaxNameLen)
		}
		slug := Slugify(value)
		if slug == "" {
			return nil, nil, fmt.Errorf("tags: %q holds no letter or digit, so it names no label", value)
		}
		names = append(names, value)
		slugs = append(slugs, slug)
	}
	return distinct(names), distinct(slugs), nil
}

// likeTerm lowercases a search term and makes its wildcards literal.
//
// The escape character is escaped first. Doing it last would escape the
// backslashes this function had just written and turn every wildcard back into
// one.
func likeTerm(term string) string {
	var b strings.Builder
	b.Grow(len(term))
	for _, r := range strings.ToLower(term) {
		if r == likeEscape || r == '%' || r == '_' {
			b.WriteRune(likeEscape)
		}
		b.WriteRune(r)
	}
	return b.String()
}
