package tags

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// The shape an owner kind's name is held to.
//
// The characters are the ones a tenant is held to, for the same reason: the
// name is concatenated with a version behind a separator and stored in one
// column, so a name carrying the separator would name a different kind on the
// way back out.
const (
	// MaxOwnerNameLen is the longest an owner kind's name may be.
	MaxOwnerNameLen = 64
	// MaxOwnerVersion is the highest version an owner kind may declare. It
	// exists so the stored value has a bounded length, and because a kind on
	// its thousandth revision is a modelling problem rather than a versioning
	// one.
	MaxOwnerVersion = 999
	// versionSeparator divides the name from the version in the stored value.
	// It is not a legal character in a name, so reading one back is
	// unambiguous.
	versionSeparator = ".v"
)

// ErrOwnerType is returned when a value cannot be an owner kind. It is the
// sentinel every refusal below wraps, so a caller tests for the kind of
// failure rather than for a sentence.
var ErrOwnerType = errors.New("tags: invalid owner type")

// OwnerType identifies a kind of entity that can carry tags.
//
// It is declared by the domain that owns the entity, and it is never derived
// from the entity's Go type. A Go type name changes when its package is
// renamed, moved, aliased or vendored, and none of those changes touch the
// rows already stored: every association written under the old name would go
// on naming a kind that nothing answers to any more, and nothing would report
// it. What is written into a column has to be a fact about the domain, chosen
// once and kept, so it is chosen by hand.
//
// It carries a version for the same reason. When the meaning of a kind changes
// -- its identifiers are reissued, its rows are migrated into a different set
// -- the next version is a different kind, and the associations written
// against the previous one stay attached to it instead of silently naming rows
// they were never about.
//
// The zero value names no kind and is refused everywhere it is accepted.
type OwnerType struct {
	name    string
	version int
}

// MustOwnerType returns the owner kind, and panics when it cannot be one.
//
// It is for the package-level declaration each owning domain writes once:
//
//	var ArticleType = tags.MustOwnerType("article", 1)
//
// There the argument is a literal, so a value that cannot be a kind is a
// mistake in the source rather than input, and the program that holds it
// cannot run correctly. Reading a kind back out of storage or configuration is
// ParseOwnerType, which reports rather than panics.
func MustOwnerType(name string, version int) OwnerType {
	t, err := newOwnerType(name, version)
	if err != nil {
		panic(err.Error())
	}
	return t
}

// ParseOwnerType reads back a kind written by String.
//
// It is what turns a stored owner_type into a value again, and it validates
// what it reads: a row written by an older version of an application, or by
// hand, is not trusted to hold a name this package would have accepted.
func ParseOwnerType(stored string) (OwnerType, error) {
	name, rawVersion, found := strings.Cut(stored, versionSeparator)
	if !found {
		return OwnerType{}, fmt.Errorf("%w: %q carries no %q version suffix", ErrOwnerType, stored, versionSeparator)
	}
	version, err := strconv.Atoi(rawVersion)
	if err != nil {
		return OwnerType{}, fmt.Errorf("%w: %q does not end in a version number", ErrOwnerType, stored)
	}
	return newOwnerType(name, version)
}

// newOwnerType is the one place a kind is checked, so MustOwnerType and
// ParseOwnerType cannot come to disagree about what a kind may be.
func newOwnerType(name string, version int) (OwnerType, error) {
	switch {
	case name == "":
		return OwnerType{}, fmt.Errorf("%w: the name is empty", ErrOwnerType)
	case len(name) > MaxOwnerNameLen:
		return OwnerType{}, fmt.Errorf("%w: the name %q is longer than %d characters", ErrOwnerType, name, MaxOwnerNameLen)
	case version < 1 || version > MaxOwnerVersion:
		return OwnerType{}, fmt.Errorf("%w: the version of %q is %d, and has to be between 1 and %d", ErrOwnerType, name, version, MaxOwnerVersion)
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		lower := c >= 'a' && c <= 'z'
		digit := c >= '0' && c <= '9'
		punctuation := c == '-' || c == '_'
		// A digit or punctuation first would make a name that reads as
		// something other than a kind, and would sort apart from its siblings.
		if !lower && !digit && !punctuation || (i == 0 && !lower) {
			return OwnerType{}, fmt.Errorf("%w: the name %q may hold lowercase letters, digits, - and _, and has to start with a letter", ErrOwnerType, name)
		}
	}
	return OwnerType{name: name, version: version}, nil
}

// Name is the kind without its version.
func (t OwnerType) Name() string { return t.name }

// Version is the revision of the kind.
func (t OwnerType) Version() int { return t.version }

// IsZero reports whether this names no kind at all.
func (t OwnerType) IsZero() bool { return t.name == "" }

// String is the value stored in the owner_type column, which ParseOwnerType
// reads back. The zero value renders as the empty string, which is not a kind
// and is refused rather than written.
func (t OwnerType) String() string {
	if t.IsZero() {
		return ""
	}
	return t.name + versionSeparator + strconv.Itoa(t.version)
}

// MaxOwnerIDLen is the longest an owner's identifier may be. It is generous
// enough for a uuid, an integer written out, or a slug, and bounded because
// the value takes part in a key.
const MaxOwnerIDLen = 128

// OwnerRef names one entity that carries tags: its kind, and its identifier
// within that kind.
//
// Its fields are unexported and Ref is the only way to build one, so a value
// that reached this package has already been checked. The zero value names
// nothing and every method that takes one refuses it.
type OwnerRef struct {
	kind OwnerType
	id   string
}

// Ref names the entity of this kind with this identifier.
//
// The identifier is the one the owning domain uses for the row, and this
// package neither reads it nor gives it meaning: it stores it, and hands it
// back when asked which entities carry a tag.
func Ref(kind OwnerType, id string) (OwnerRef, error) {
	switch {
	case kind.IsZero():
		return OwnerRef{}, fmt.Errorf("%w: the kind is the zero value, which names nothing", ErrOwnerType)
	case id == "":
		return OwnerRef{}, fmt.Errorf("%w: the identifier of a %s is empty", ErrOwnerType, kind)
	case len(id) > MaxOwnerIDLen:
		return OwnerRef{}, fmt.Errorf("%w: the identifier of a %s is longer than %d characters", ErrOwnerType, kind, MaxOwnerIDLen)
	}
	return OwnerRef{kind: kind, id: id}, nil
}

// Type is the kind of entity this refers to.
func (r OwnerRef) Type() OwnerType { return r.kind }

// ID is the entity's identifier within its kind.
func (r OwnerRef) ID() string { return r.id }

// IsZero reports whether this refers to no entity at all.
func (r OwnerRef) IsZero() bool { return r.kind.IsZero() || r.id == "" }

// String is the reference as one readable value, for a log line or an error.
func (r OwnerRef) String() string {
	if r.IsZero() {
		return ""
	}
	return r.kind.String() + ":" + r.id
}
