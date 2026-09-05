package tags

import (
	"fmt"
	"net/http"

	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/translation"
)

// The defaults for the optional settings. They are constants rather than
// literals inside Config.withDefaults, so the value a reader finds here is the
// value the package uses.
const (
	// DefaultPrefix is where the routes are mounted when Config leaves Prefix
	// empty.
	DefaultPrefix = "/tags"
	// DefaultPageSize is how many records one page answers with when Config
	// leaves PageSize at zero.
	DefaultPageSize = 25
	// MaxPageSize is the ceiling PageSize is refused above. A page nobody
	// bounded is a page that reads the whole table on the day the table is
	// large.
	MaxPageSize = 200
)

// Config is what the application passes when it wires this package.
//
// A typed struct rather than a map: a misspelled key in a map is a setting that
// silently keeps its default, and the failure shows up as behaviour nobody
// asked for rather than as an error. Here a field that does not exist does not
// compile.
type Config struct {
	// Tenant is the customer a visitor with no session is read as.
	//
	// It is required, and it comes from the application's own configuration --
	// never from the request. A tenant a visitor could name is a visitor who
	// chooses whose rows they read. Everywhere there is a session, the tenant
	// comes from the Grant instead, and this value is not consulted at all.
	Tenant string

	// Prefix is the path the routes are mounted under. Empty means
	// DefaultPrefix.
	Prefix string

	// PageSize is how many records one page answers with. Zero means
	// DefaultPageSize, and anything above MaxPageSize is refused rather than
	// clamped: a number somebody wrote and did not get is worse than a number
	// somebody wrote and was told about.
	PageSize int

	// CSRF issues the token every form on these screens carries.
	//
	// It is required, because every screen here writes: a page rendered without
	// a token is a page whose buttons the application refuses, and finding that
	// out from a form that does nothing is worse than finding it out at boot.
	CSRF *security.CSRF

	// Translator is the application's own catalogue, asked before the one this
	// package ships.
	//
	// It is optional. Nothing is asked of it when it is nil, and the screens are
	// drawn in the locales this package carries -- which is what an application
	// that renders in one language would have got anyway.
	Translator *translation.Translator

	// Policy decides who may do what with a Tag.
	//
	// Nil is TagPolicy, which denies everything -- so a wiring that says nothing
	// about rules gets no access at all, which is the state to start from.
	//
	// It is here because an application installs this package with `go get` and
	// cannot edit the policy inside it. Without this field the only way to open
	// an action would be to fork the package, and a forked security boundary is
	// one that stops receiving the fixes made to the original. There is still
	// exactly one policy, consulted in exactly one place: what this changes is
	// who writes it, not how many of them there are.
	//
	// The rules go in the application, beside the rest of its authorization:
	//
	//	type TagRules struct{}
	//
	//	func (TagRules) Can(_ context.Context, s security.Subject, a security.Action, record tags.Tag) error {
	//		if record.ID != "" && record.TenantID != s.Tenant {
	//			return fmt.Errorf("the tag belongs to another tenant")
	//		}
	//		if !s.HasRole(string(a)) {
	//			return fmt.Errorf("the subject does not carry %s", a)
	//		}
	//		return nil
	//	}
	//
	// The tenant comparison is the line to keep whatever else is written: every
	// rule below it holds across customers without it, as soon as two of them
	// have a record with the same identifier.
	Policy security.Policy[Tag]
}

// Validate reports what the configuration cannot be used with.
//
// It is called by New, so an application with a setting that cannot work fails
// where it is wired rather than on the first request that needed it.
func (c Config) Validate() error {
	if c.Tenant == "" {
		return fmt.Errorf("tags: Config.Tenant is required: a visitor with no session has to be read as some customer, and it cannot be one the request names")
	}
	// The same rule the framework applies to every tenant it accepts. A tenant
	// is concatenated into a storage path, a cache key and a lock name, so one
	// carrying a separator lands in another tenant's namespace.
	if !security.ValidTenant(c.Tenant) {
		return fmt.Errorf("tags: Config.Tenant is %q, which cannot be a tenant: lowercase letters, digits, - and _, up to 64 characters", c.Tenant)
	}
	if c.Prefix != "" && c.Prefix[0] != '/' {
		return fmt.Errorf("tags: Config.Prefix is %q and has to start with /", c.Prefix)
	}
	if c.Prefix != "" {
		if err := validateRoutePrefix(c.Prefix); err != nil {
			return err
		}
	}
	if c.PageSize < 0 || c.PageSize > MaxPageSize {
		return fmt.Errorf("tags: Config.PageSize is %d, and has to be between 0 and %d, where 0 means %d", c.PageSize, MaxPageSize, DefaultPageSize)
	}
	if c.CSRF == nil {
		return fmt.Errorf("tags: Config.CSRF is required: every screen this module draws writes, and a form with no token is a form the application refuses")
	}
	return nil
}

// validateRoutePrefix asks the standard library to parse the exact patterns
// the module will register. Its parser is not exported and reports invalid
// patterns by panic, so the throwaway mux turns that boot-time panic into the
// configuration error New promises.
func validateRoutePrefix(prefix string) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("tags: Config.Prefix %q cannot be registered as a route path", prefix)
		}
	}()

	handler := http.NotFoundHandler()
	mux := http.NewServeMux()
	for _, route := range routePatterns(prefix) {
		mux.Handle(route.method+" "+route.pattern, handler)
	}
	return nil
}

// route is one line of the table this module registers.
type route struct {
	method  string
	pattern string
	name    string
}

// routePatterns is the whole table, built from one prefix.
//
// It is one list read twice -- by Validate, which registers it on a throwaway
// mux to find out whether the standard library will take it, and by Routes,
// which registers it for real. Two lists would be two tables, and the one the
// configuration was checked against would not be the one that serves.
func routePatterns(prefix string) []route {
	return []route{
		{http.MethodGet, prefix, "tags.index"},
		{http.MethodPost, prefix, "tags.store"},
		// The literal segment is registered before the wildcard for a reader's
		// sake only: the standard library prefers the more specific pattern
		// whatever order they arrive in.
		{http.MethodGet, prefix + "/order", "tags.order"},
		{http.MethodPost, prefix + "/order", "tags.reorder"},
		{http.MethodGet, prefix + "/{id}", "tags.show"},
		{http.MethodPatch, prefix + "/{id}", "tags.update"},
		{http.MethodDelete, prefix + "/{id}", "tags.destroy"},
		{http.MethodPost, prefix + "/{id}/move", "tags.move"},
	}
}

// withDefaults returns the configuration with the optional fields filled in.
//
// It runs after Validate and never before: filling a default in first would
// hide the value somebody actually wrote from the check that would have refused
// it.
func (c Config) withDefaults() Config {
	if c.Prefix == "" {
		c.Prefix = DefaultPrefix
	}
	if c.PageSize == 0 {
		c.PageSize = DefaultPageSize
	}
	return c
}
