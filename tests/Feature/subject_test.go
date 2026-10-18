package feature_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/arandu-io/framework/data"
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/security"

	tags "github.com/hyz-is/arandu-tags"
)

// These tests hold Config.Subject: the routes of this module act as the
// subject the application builds, under that subject's tenant, instead of the
// one the realm session holds.
//
// An application with organizations signs a person in once and has them act
// inside the organization they selected, so the tenant the routes have to read
// and write is the organization's -- which the session store this package is
// handed does not know about.

// memberRules lets a subject with an identifier list, read and create labels
// of its own tenant, and refuses everybody else, guests included. It keeps the
// subjects it was asked about, so a test can say who the routes acted as.
type memberRules struct {
	mu    *sync.Mutex
	asked *[]security.Subject
}

func newMemberRules() memberRules {
	return memberRules{mu: new(sync.Mutex), asked: new([]security.Subject)}
}

func (p memberRules) Can(_ context.Context, s security.Subject, a security.Action, record tags.Tag) error {
	p.mu.Lock()
	*p.asked = append(*p.asked, s)
	p.mu.Unlock()

	if record.ID != "" && record.TenantID != s.Tenant {
		return fmt.Errorf("the tag belongs to another tenant")
	}
	if s.ID == "" {
		return fmt.Errorf("%s is not open to a visitor", a)
	}
	switch a {
	case tags.TagList, tags.TagView, tags.TagCreate:
		return nil
	}
	return fmt.Errorf("%s is not open", a)
}

// lastAsked is the subject the policy was asked about most recently.
func (p memberRules) lastAsked(t *testing.T) security.Subject {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(*p.asked) == 0 {
		t.Fatal("the policy was never asked")
	}
	return (*p.asked)[len(*p.asked)-1]
}

// organizationMember is the subject an application builds for a member acting
// inside the organization they selected: the organization is the tenant.
var organizationMember = security.Subject{ID: "member-1", Tenant: "globex", Verified: true}

// routed mounts the module's routes on a bare router, the way the route tests
// do, over a migrated database.
func routed(t *testing.T, cfg tags.Config) (*tags.Module, http.Handler) {
	t.Helper()
	module, _ := migratedWith(t, cfg)
	router := fhttp.NewRouter()
	module.Routes(router.ForModule(module.Name()))
	return module, router
}

// asJSON makes one request that asks for the resource rather than the screen.
func asJSON(handler http.Handler, method, target string, form url.Values) *httptest.ResponseRecorder {
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestTheRoutesActAsTheSubjectTheApplicationBuilds(t *testing.T) {
	t.Parallel()

	rules := newMemberRules()
	module, handler := routed(t, tags.Config{
		Tenant: "acme",
		Policy: rules,
		Subject: func(*http.Request) (security.Subject, error) {
			return organizationMember, nil
		},
	})

	// A write through the route lands in the organization's tenant.
	rec := asJSON(handler, http.MethodPost, tags.DefaultPrefix, url.Values{"name": {"Urgent"}, "type": {"topic"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("creating through the route answered %d: %s", rec.Code, rec.Body)
	}
	if got := rules.lastAsked(t); got.ID != organizationMember.ID || got.Tenant != organizationMember.Tenant {
		t.Fatalf("the route acted as %+v, want the subject the application built", got)
	}
	held, err := module.Service().List(context.Background(), organizationMember, "topic", data.Query{Limit: tags.MaxPageSize})
	if err != nil || len(held) != 1 || held[0].TenantID != "globex" {
		t.Fatalf("the organization holds %v (%v), want the one label, written for globex", held, err)
	}

	// And a read through the route is scoped by the same tenant.
	rec = asJSON(handler, http.MethodGet, tags.DefaultPrefix+"?type=topic", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("listing through the route answered %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the listing is not JSON: %v\n%s", err, rec.Body)
	}
	if len(body.Data.Items) != 1 || body.Data.Items[0]["name"] != "Urgent" {
		t.Fatalf("the listing answered %v, want the organization's label", body.Data.Items)
	}
}

func TestWithoutTheHookTheRoutesReadTheSession(t *testing.T) {
	t.Parallel()

	rules := newMemberRules()
	_, handler := routed(t, tags.Config{Tenant: "acme", Policy: rules})

	// No session and no hook: a guest of the configured tenant, as before.
	rec := asJSON(handler, http.MethodGet, tags.DefaultPrefix+"?type=topic", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a visitor answered %d, want %d", rec.Code, http.StatusForbidden)
	}
	if got := rules.lastAsked(t); !got.IsGuest() || got.Tenant != "acme" {
		t.Fatalf("the route asked about %+v, want a guest of acme", got)
	}
}

func TestTheHookDecidesWhatAVisitorAndAFailureAre(t *testing.T) {
	t.Parallel()

	failure := errors.New("the membership could not be read")
	for _, c := range []struct {
		why   string
		hook  func(*http.Request) (security.Subject, error)
		want  func(int) bool
		guest string
	}{
		{
			why:   "the empty subject is a guest of the configured tenant",
			hook:  func(*http.Request) (security.Subject, error) { return security.Subject{}, nil },
			want:  func(code int) bool { return code == http.StatusForbidden },
			guest: "acme",
		},
		{
			why:   "a declared guest keeps the tenant it names",
			hook:  func(*http.Request) (security.Subject, error) { return security.Guest("globex"), nil },
			want:  func(code int) bool { return code == http.StatusForbidden },
			guest: "globex",
		},
		{
			why:  "a refusal is a 403",
			hook: func(*http.Request) (security.Subject, error) { return security.Subject{}, security.ErrForbidden },
			want: func(code int) bool { return code == http.StatusForbidden },
		},
		{
			why:  "any other failure is the framework's error and not a visitor",
			hook: func(*http.Request) (security.Subject, error) { return security.Subject{}, failure },
			want: func(code int) bool { return code >= http.StatusInternalServerError },
		},
	} {
		rules := newMemberRules()
		_, handler := routed(t, tags.Config{Tenant: "acme", Policy: rules, Subject: c.hook})

		rec := asJSON(recovering(t, handler, failure), http.MethodGet, tags.DefaultPrefix+"?type=topic", nil)
		if !c.want(rec.Code) {
			t.Errorf("%s: the listing answered %d", c.why, rec.Code)
			continue
		}
		if c.guest == "" {
			if len(*rules.asked) != 0 {
				t.Errorf("%s: the policy was asked about %v, and no subject should have reached it", c.why, *rules.asked)
			}
			continue
		}
		if got := rules.lastAsked(t); !got.IsGuest() || got.Tenant != c.guest {
			t.Errorf("%s: the route asked about %+v, want a guest of %s", c.why, got, c.guest)
		}
	}
}

// recovering stands in for the framework's recovery middleware, which a bare
// router does not mount: an error a handler returns and the router has no
// status for is raised, and the application turns it into its error page. Only
// the expected failure is answered as that page; anything else is raised again.
func recovering(t *testing.T, next http.Handler, expected error) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			if err, ok := recovered.(error); ok && errors.Is(err, expected) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			panic(recovered)
		}()
		next.ServeHTTP(w, r)
	})
}

func TestNewNeedsNoSessionStoreWhenTheApplicationBuildsTheSubject(t *testing.T) {
	t.Parallel()

	hook := func(*http.Request) (security.Subject, error) { return organizationMember, nil }
	if _, err := tags.New(tags.Config{Tenant: "acme", Subject: hook}, data.Wrap(nil, data.DialectSQLite), nil); err != nil {
		t.Fatalf("a wiring with Config.Subject and no session store was refused: %v", err)
	}
	if _, err := tags.New(tags.Config{Tenant: "acme"}, data.Wrap(nil, data.DialectSQLite), nil); err == nil {
		t.Fatal("a wiring with neither a session store nor Config.Subject was accepted")
	}
}
