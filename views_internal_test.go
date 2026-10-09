package tags

import (
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fhttp "github.com/arandu-io/framework/http"
)

// The addresses a screen writes are composed here, whole, and the markup writes
// each as one value. That moves the escaping into Go as well: the route table
// fills a parameter with what it is handed and the view no longer composes
// anything, so an identifier or a taxonomy that is not escaped here reaches the
// page as a different address.
//
// The probe is a route on the same router the module registers on, so the
// addresses come from the route table a request is served with and not from a
// second spelling of the prefix.
func TestTheAddressesAScreenWritesAreComposedWholeAndEscaped(t *testing.T) {
	t.Parallel()

	m := &Module{cfg: Config{Prefix: "/labels"}.withDefaults()}
	router := fhttp.NewRouter()
	m.Routes(router)

	var got []string
	router.Action(stdhttp.MethodGet, "/probe", func(ctx *fhttp.Context) error {
		rows := m.linked(ctx, []Row{{ID: "a b/c?d"}})
		got = []string{rows[0].URL, rows[0].MoveURL, m.listingOf(ctx, "x&y z"), m.orderingOf(ctx, "x&y z")}
		return ctx.Status(stdhttp.StatusNoContent)
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(stdhttp.MethodGet, "/probe", nil))

	want := []string{
		"/labels/a%20b%2Fc%3Fd",
		"/labels/a%20b%2Fc%3Fd/move",
		"/labels?type=x%26y+z",
		"/labels/order?type=x%26y+z",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("the addresses a screen writes are\n\t%s\nwant\n\t%s", strings.Join(got, "\n\t"), strings.Join(want, "\n\t"))
	}
}
