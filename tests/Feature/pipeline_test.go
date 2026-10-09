package feature_test

import (
	"context"
	"database/sql"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/http/middleware"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/view"

	tags "github.com/hyz-is/arandu-tags"

	_ "modernc.org/sqlite"
)

// These tests serve the screens the way an application does: behind the
// middleware that protects forms, over a migrated database, with a policy that
// lets a visitor write. What they hold is the round trip a person makes -- load
// a screen, submit the form it drew -- and the requests that must still be
// turned away.
//
// The renderer below stands in for the application's layout, and it draws what
// that layout draws from the page: the brand link, the sign-in link while nobody
// is signed in, and the hidden _token field @csrf writes. It reads them through
// view.Layout, the interface the layout itself is compiled against, so a field
// the layout would show empty is shown empty here.

// layout is the application's layout, reduced to the parts these tests read.
type layout struct{}

func (layout) Render(_ context.Context, w http.ResponseWriter, status int, name string, data any) error {
	page, ok := data.(view.Layout)
	if !ok {
		return fmt.Errorf("%s was handed %T, which the layout cannot draw", name, data)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	var b strings.Builder
	fmt.Fprintf(&b, "<a data-brand href=\"%s\">%s</a>\n", html.EscapeString(page.HomeLink()), html.EscapeString(page.BrandName()))
	if !page.SignedIn() {
		fmt.Fprintf(&b, "<a data-login href=\"%s\">Sign in</a>\n", html.EscapeString(page.LoginLink()))
	}
	fmt.Fprintf(&b, "<form method=\"post\"><input type=\"hidden\" name=\"_token\" value=\"%s\"></form>\n", html.EscapeString(page.CSRFToken()))
	_, err := w.Write([]byte(b.String()))
	return err
}

// visitorRules lets anybody of the tenant list, read and create labels,
// visitors included. It is the policy of an application that means its
// taxonomy to be open, and the one a guest form needs to reach a write at all.
type visitorRules struct{}

var _ security.Policy[tags.Tag] = visitorRules{}

func (visitorRules) Can(_ context.Context, s security.Subject, a security.Action, record tags.Tag) error {
	if record.ID != "" && record.TenantID != s.Tenant {
		return fmt.Errorf("the tag belongs to another tenant")
	}
	switch a {
	case tags.TagList, tags.TagView, tags.TagCreate:
		return nil
	}
	return fmt.Errorf("%s is not open", a)
}

// application is one served instance: the handler the server would run, the
// session store a sign-in writes to, and the module behind both.
type application struct {
	handler  http.Handler
	sessions *security.SessionStore
	module   *tags.Module
}

// databases numbers the in-memory databases the applications below open.
var databases atomic.Int64

// serve builds the application. withNavigation registers the two routes the
// layout links to, under the names the application skeleton gives them; without
// it the application has neither, which is the case where no link is the right
// answer.
func serve(t *testing.T, withNavigation bool) application {
	t.Helper()

	// A database of its own per application, so two served in one test do not
	// share rows or a schema.
	name := fmt.Sprintf("%s_%d", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()), databases.Add(1))
	raw, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	raw.SetMaxOpenConns(1)

	sessions := security.NewSessionStore([]byte(appKey), time.Hour, false, security.NewMemoryBackend())
	module, err := tags.New(tags.Config{Tenant: "acme", Policy: visitorRules{}}, data.Wrap(raw, data.DialectSQLite), sessions)
	if err != nil {
		t.Fatalf("building the module: %v", err)
	}

	// In an application this is `aru migrate`, run before it serves.
	migrator := database.ForMigrations(database.NewConnection(raw, "", "", map[string]any{
		"driver": string(database.DialectSQLite),
		"name":   name,
	}))
	for _, migration := range module.Migrations() {
		if err := migration.Up(context.Background(), migrator); err != nil {
			t.Fatalf("applying %s: %v", migration.GetName(), err)
		}
	}

	router := fhttp.NewRouter().WithRenderer(layout{})
	if withNavigation {
		nothing := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
		router.Get("/{$}", nothing).Name("home")
		router.Get("/auth/login", nothing).Name("auth.login")
	}
	module.Routes(router.ForModule(module.Name()))

	// The middleware the application skeleton mounts, built the way it builds
	// it: over the session store's own reader of the session cookie.
	csrf := security.NewCSRF([]byte(appKey), time.Hour).Secure(false)
	return application{
		handler:  middleware.CSRFProtect(csrf, sessions.IDFromRequest)(router),
		sessions: sessions,
		module:   module,
	}
}

// visit makes one request carrying the cookies given, and returns the answer.
func (a application) visit(t *testing.T, method, target string, form url.Values, cookies []*http.Cookie, header http.Header) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	for key, values := range header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)
	return rec
}

// screen loads the listing and answers the token its form carries and the
// cookies the answer set, as a browser would keep them.
func (a application) screen(t *testing.T, cookies []*http.Cookie) (string, []*http.Cookie) {
	t.Helper()

	rec := a.visit(t, http.MethodGet, tags.DefaultPrefix+"?type=topic", nil, cookies, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("the listing answered %d: %s", rec.Code, rec.Body)
	}
	return field(t, rec.Body.String(), "_token"), append(append([]*http.Cookie(nil), cookies...), rec.Result().Cookies()...)
}

// field is the value of the hidden input called name in a drawn screen.
func field(t *testing.T, body, name string) string {
	t.Helper()
	m := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `" value="([^"]*)"`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("the screen draws no %s field:\n%s", name, body)
	}
	return html.UnescapeString(m[1])
}

// href is the target of the link marked with attr in a drawn screen, and
// whether the screen drew that link at all.
func href(body, attr string) (string, bool) {
	m := regexp.MustCompile(`<a ` + regexp.QuoteMeta(attr) + ` href="([^"]*)"`).FindStringSubmatch(body)
	if m == nil {
		return "", false
	}
	return html.UnescapeString(m[1]), true
}

// labels counts the labels of the taxonomy the forms write to.
func (a application) labels(t *testing.T) int {
	t.Helper()
	records, err := a.module.Service().List(context.Background(), security.Guest("acme"), "topic", data.Query{Limit: tags.MaxPageSize})
	if err != nil {
		t.Fatalf("listing what the forms wrote: %v", err)
	}
	return len(records)
}

func submission(name, token string) url.Values {
	form := url.Values{"name": {name}, "type": {"topic"}}
	if token != "" {
		form.Set("_token", token)
	}
	return form
}

func TestAGuestSubmitsTheFormTheScreenDrew(t *testing.T) {
	t.Parallel()

	app := serve(t, true)
	token, cookies := app.screen(t, nil)
	if token == "" {
		t.Fatal("the screen drew an empty token for a visitor with no session, so every form on it is refused")
	}

	rec := app.visit(t, http.MethodPost, tags.DefaultPrefix, submission("Go", token), cookies, nil)
	if rec.Code < 200 || rec.Code >= 400 {
		t.Fatalf("the form the screen drew answered %d, want a success or a redirect: %s", rec.Code, rec.Body)
	}
	if got := app.labels(t); got != 1 {
		t.Fatalf("the accepted form wrote %d label(s), want 1", got)
	}
}

func TestAForgedSubmissionIsStillRefused(t *testing.T) {
	t.Parallel()

	app := serve(t, true)
	token, cookies := app.screen(t, nil)
	if token == "" {
		t.Fatal("the screen drew no token, so the refusals below would prove nothing about one")
	}
	// Another visitor's browser, holding a token of its own.
	_, stranger := app.screen(t, nil)

	for _, forged := range []struct {
		why     string
		form    url.Values
		cookies []*http.Cookie
		header  http.Header
		want    int
	}{
		{"no token", submission("Rust", ""), cookies, nil, middleware.StatusCSRFExpired},
		{"a token and not the browser it was issued to", submission("Rust", token), nil, nil, middleware.StatusCSRFExpired},
		{"a token issued to another browser", submission("Rust", token), stranger, nil, middleware.StatusCSRFExpired},
		{"a token that was altered", submission("Rust", token+"x"), cookies, nil, middleware.StatusCSRFExpired},
		{"a page of another site", submission("Rust", token), cookies, http.Header{"Sec-Fetch-Site": {"cross-site"}}, http.StatusForbidden},
	} {
		rec := app.visit(t, http.MethodPost, tags.DefaultPrefix, forged.form, forged.cookies, forged.header)
		if rec.Code != forged.want {
			t.Errorf("a submission with %s answered %d, want %d", forged.why, rec.Code, forged.want)
		}
	}
	if got := app.labels(t); got != 0 {
		t.Fatalf("the forged submissions wrote %d label(s), want none", got)
	}
}

func TestASignedInSubjectSubmitsTheFormTheScreenDrew(t *testing.T) {
	t.Parallel()

	app := serve(t, true)
	signIn := httptest.NewRecorder()
	if _, err := app.sessions.Start(context.Background(), signIn, security.Subject{ID: "user-1", Tenant: "acme", Verified: true}); err != nil {
		t.Fatalf("signing in: %v", err)
	}
	session := signIn.Result().Cookies()

	rec := app.visit(t, http.MethodGet, tags.DefaultPrefix+"?type=topic", nil, session, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("the listing answered %d: %s", rec.Code, rec.Body)
	}
	if _, drawn := href(rec.Body.String(), "data-login"); drawn {
		t.Error("the screen offers a signed-in subject the sign-in link")
	}
	token := field(t, rec.Body.String(), "_token")
	if token == "" {
		t.Fatal("the screen drew an empty token for a signed-in subject")
	}

	// A guest's token is bound to a guest, and is refused on the session.
	guestToken, _ := app.screen(t, nil)
	if rec := app.visit(t, http.MethodPost, tags.DefaultPrefix, submission("Rust", guestToken), session, nil); rec.Code != middleware.StatusCSRFExpired {
		t.Errorf("a guest's token on a session answered %d, want %d", rec.Code, middleware.StatusCSRFExpired)
	}

	rec = app.visit(t, http.MethodPost, tags.DefaultPrefix, submission("Go", token), session, nil)
	if rec.Code < 200 || rec.Code >= 400 {
		t.Fatalf("the form the screen drew answered %d, want a success or a redirect: %s", rec.Code, rec.Body)
	}
	if got := app.labels(t); got != 1 {
		t.Fatalf("the accepted form wrote %d label(s), want 1", got)
	}
}

func TestTheLayoutLinksWhereTheApplicationRegisteredItsRoutes(t *testing.T) {
	t.Parallel()

	app := serve(t, true)
	for _, target := range []string{
		tags.DefaultPrefix + "?type=topic",
		tags.DefaultPrefix + "/order?type=topic",
	} {
		rec := app.visit(t, http.MethodGet, target, nil, nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s answered %d: %s", target, rec.Code, rec.Body)
		}
		body := rec.Body.String()
		if got, _ := href(body, "data-brand"); got != "/" {
			t.Errorf("%s links the brand to %q, want the route named home, /", target, got)
		}
		if got, _ := href(body, "data-login"); got != "/auth/login" {
			t.Errorf("%s links Sign in to %q, want the route named auth.login, /auth/login", target, got)
		}
	}

	// An application that registered neither route gets no address made up for
	// it: the module reads the route table and does not guess at paths.
	bare := serve(t, false)
	rec := bare.visit(t, http.MethodGet, tags.DefaultPrefix+"?type=topic", nil, nil, nil)
	if got, _ := href(rec.Body.String(), "data-brand"); got != "" {
		t.Errorf("with no route named home the brand links to %q, want nothing", got)
	}
	if got, _ := href(rec.Body.String(), "data-login"); got != "" {
		t.Errorf("with no route named auth.login Sign in links to %q, want nothing", got)
	}
}
