package feature_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/security"

	tags "github.com/hyz-is/arandu-tags"
)

// The colour of a label through the routes: the forms send it, the resource
// answers with it, and a rename that does not send it leaves it alone.

// colourRules lets anybody of the tenant do everything to a label, which is
// what these tests need to reach every route; the tenant comparison stays.
type colourRules struct{}

func (colourRules) Can(_ context.Context, s security.Subject, _ security.Action, record tags.Tag) error {
	if record.ID != "" && record.TenantID != s.Tenant {
		return security.ErrForbidden
	}
	return nil
}

// capture is a renderer that keeps what each screen was handed instead of
// drawing it, so a test reads the page data the published markup draws from.
type capture struct {
	mu   *sync.Mutex
	data *[]any
}

func (c capture) Render(_ context.Context, w http.ResponseWriter, status int, _ string, data any) error {
	c.mu.Lock()
	*c.data = append(*c.data, data)
	c.mu.Unlock()
	w.WriteHeader(status)
	return nil
}

func (c capture) last(t *testing.T) any {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(*c.data) == 0 {
		t.Fatal("no screen was drawn")
	}
	return (*c.data)[len(*c.data)-1]
}

// coloured serves the module with these colours, behind a renderer that
// captures, acting as one member of acme.
func coloured(t *testing.T, colours ...string) (http.Handler, capture) {
	t.Helper()
	member := security.Subject{ID: "member-1", Tenant: "acme", Verified: true}
	module, _ := migratedWith(t, tags.Config{
		Tenant:  "acme",
		Policy:  colourRules{},
		Colours: colours,
		Subject: func(*http.Request) (security.Subject, error) { return member, nil },
	})
	screens := capture{mu: new(sync.Mutex), data: new([]any)}
	router := fhttp.NewRouter().WithRenderer(screens)
	module.Routes(router.ForModule(module.Name()))
	return router, screens
}

// resource decodes one resource answer.
func resource(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not a resource: %v\n%s", err, rec.Body)
	}
	return body.Data
}

// patch renames through the route, sending the form given.
func patch(handler http.Handler, id string, form url.Values) *httptest.ResponseRecorder {
	return asJSON(handler, http.MethodPatch, tags.DefaultPrefix+"/"+url.PathEscape(id), form)
}

func TestTheRoutesCarryTheColourOfALabel(t *testing.T) {
	t.Parallel()

	handler, _ := coloured(t, "red", "green")

	rec := asJSON(handler, http.MethodPost, tags.DefaultPrefix, url.Values{"name": {"Urgent"}, "type": {"status"}, "colour": {"red"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("creating a red label answered %d: %s", rec.Code, rec.Body)
	}
	created := resource(t, rec)
	if created["colour"] != "red" {
		t.Fatalf("the created label answers colour %v, want red", created["colour"])
	}
	id, _ := created["id"].(string)

	// A rename that sends no colour -- a form published before colours, or a
	// client that only renames -- leaves it alone.
	if rec := patch(handler, id, url.Values{"name": {"Very urgent"}}); rec.Code != http.StatusOK || resource(t, rec)["colour"] != "red" {
		t.Fatalf("a rename without a colour answered %d %s, want red kept", rec.Code, rec.Body)
	}
	if rec := patch(handler, id, url.Values{"name": {"Very urgent"}, "colour": {"green"}}); rec.Code != http.StatusOK || resource(t, rec)["colour"] != "green" {
		t.Fatalf("a rename to green answered %d %s", rec.Code, rec.Body)
	}
	if rec := patch(handler, id, url.Values{"name": {"Very urgent"}, "colour": {""}}); rec.Code != http.StatusOK || resource(t, rec)["colour"] != "" {
		t.Fatalf("a rename to no colour answered %d %s", rec.Code, rec.Body)
	}

	// The same holds for a JSON body: the field is changed only when it was
	// sent.
	for _, c := range []struct {
		body string
		want string
	}{
		{`{"name": "Very urgent", "colour": "red"}`, "red"},
		{`{"name": "Very urgent"}`, "red"},
		{`{"name": "Very urgent", "colour": ""}`, ""},
	} {
		req := httptest.NewRequest(http.MethodPatch, tags.DefaultPrefix+"/"+url.PathEscape(id), strings.NewReader(c.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || resource(t, rec)["colour"] != c.want {
			t.Fatalf("a JSON rename %s answered %d %s, want colour %q", c.body, rec.Code, rec.Body, c.want)
		}
	}

	// A colour the application does not offer is a rejected input, before
	// anything is written.
	if rec := patch(handler, id, url.Values{"name": {"Very urgent"}, "colour": {"blue"}}); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "colour") {
		t.Fatalf("a rename to blue answered %d %s, want 422 naming colour", rec.Code, rec.Body)
	}
	rec = asJSON(handler, http.MethodPost, tags.DefaultPrefix, url.Values{"name": {"Other"}, "type": {"status"}, "colour": {"blue"}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("creating a blue label answered %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestTheFormsOfferTheApplicationsColours(t *testing.T) {
	t.Parallel()

	handler, screens := coloured(t, "red", "green")
	rec := asJSON(handler, http.MethodPost, tags.DefaultPrefix, url.Values{"name": {"Urgent"}, "type": {"status"}, "colour": {"green"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("creating answered %d: %s", rec.Code, rec.Body)
	}
	id, _ := resource(t, rec)["id"].(string)

	screen := httptest.NewRecorder()
	handler.ServeHTTP(screen, httptest.NewRequest(http.MethodGet, tags.DefaultPrefix+"?type=status", nil))
	index, ok := screens.last(t).(tags.IndexPageData)
	if !ok {
		t.Fatalf("the listing was handed %T", screens.last(t))
	}
	if len(index.Colours) != 2 || index.Colours[0].Token != "red" || index.Colours[1].Class != "tag--green" {
		t.Fatalf("the create form offers %+v", index.Colours)
	}
	if len(index.Rows) != 1 || index.Rows[0].ColourClass != "tag--green" {
		t.Fatalf("the listing draws %+v, want the label with the class tag--green", index.Rows)
	}

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tags.DefaultPrefix+"/"+url.PathEscape(id), nil))
	edit, ok := screens.last(t).(tags.EditPageData)
	if !ok {
		t.Fatalf("the rename screen was handed %T", screens.last(t))
	}
	if len(edit.Colours) != 2 || edit.Colours[0].Selected || !edit.Colours[1].Selected {
		t.Fatalf("the rename form offers %+v, want green chosen", edit.Colours)
	}

	// An application that offers none draws no choice.
	plain, plainScreens := coloured(t)
	plain.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tags.DefaultPrefix+"?type=status", nil))
	if got := plainScreens.last(t).(tags.IndexPageData).Colours; len(got) != 0 {
		t.Fatalf("an application with no colours is offered %+v", got)
	}
}
