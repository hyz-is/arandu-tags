package tags

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/console"
)

// The commands, run in process.
//
// They are driven through the console application rather than by calling Run
// directly, because that is what binds the signature: an argument this package
// declares and never reads, or reads under a name it never declared, is only
// visible once something has parsed the line the way the CLI parses it.

// terminal is a console holding this package's commands, over an open service,
// and the buffer everything it prints goes to.
func terminal(t *testing.T, service *TagService) (*console.Application, *bytes.Buffer) {
	t.Helper()

	var out bytes.Buffer
	app := console.NewApplication(&out, &out, strings.NewReader(""))
	commands, err := Commands(Deps{Service: service, Operator: memberOf})
	if err != nil {
		t.Fatalf("building the commands: %v", err)
	}
	app.Add(commands...)
	return app, &out
}

// run makes one invocation and returns what it said and how it ended.
//
// What it said is the output and the failure joined, because a command reports
// through both: console.Exit carries its sentence in the error the application
// returns, and everything else is written to the terminal as it happens.
func run(t *testing.T, app *console.Application, out *bytes.Buffer, args ...string) (string, int) {
	t.Helper()

	out.Reset()
	err := app.Handle(context.Background(), args)
	said := out.String()
	if err != nil {
		said += err.Error()
	}
	return said, console.ExitCode(err)
}

func TestEveryCommandIsNamedUnderThePackagePrefix(t *testing.T) {
	t.Parallel()

	commands, err := Commands(Deps{Service: openService(t, "commands-names"), Operator: memberOf})
	if err != nil {
		t.Fatalf("building the commands: %v", err)
	}
	if len(commands) == 0 {
		t.Fatal("the package ships no command, so everything below would pass by having nothing to read")
	}
	for _, command := range commands {
		// The name comes from the first word of the signature, and the
		// application registers them beside every other package's: a command
		// outside the prefix is one that can collide with a name nobody here
		// chose.
		name, _, _, err := command.Definition()
		if err != nil {
			t.Fatalf("the signature of %q does not parse: %v", command.Signature, err)
		}
		if !strings.HasPrefix(name, CommandPrefix) {
			t.Errorf("%s is not named under %s", name, CommandPrefix)
		}
		if command.Description == "" {
			t.Errorf("%s has no description, and `aru list` prints one per line", name)
		}
	}
}

func TestTheCommandsRefuseAWiringThatCannotWork(t *testing.T) {
	t.Parallel()

	if _, err := Commands(Deps{Operator: memberOf}); err == nil {
		t.Error("commands with no service were built")
	}
	if _, err := Commands(Deps{Service: openService(t, "commands-wiring")}); err == nil {
		t.Error("commands with no operator were built")
	}
}

func TestACommandNeedsToBeToldWhichCustomerItIsFor(t *testing.T) {
	t.Parallel()

	app, out := terminal(t, openService(t, "commands-tenant"))

	// There is no session here to say which customer, so the flag is the only
	// answer and a command without it refuses rather than guessing.
	if _, code := run(t, app, out, "tags:list"); code == 0 {
		t.Error("a command with no --tenant succeeded")
	}
	if _, code := run(t, app, out, "tags:list", "--tenant=Acme"); code == 0 {
		t.Error("a command with a tenant that cannot be one succeeded")
	}
}

func TestACommandWillNotReadAnotherCustomersLabels(t *testing.T) {
	t.Parallel()

	// An operator function that answers with the wrong customer is a mistake in
	// the application, and it is the one answer this package can recognise: a
	// subject of another customer would read that customer's rows under a
	// command line that named this one.
	commands, err := Commands(Deps{
		Service:  openService(t, "commands-wrong-operator"),
		Operator: func(string) security.Subject { return memberOf("globex") },
	})
	if err != nil {
		t.Fatalf("building the commands: %v", err)
	}
	var out bytes.Buffer
	app := console.NewApplication(&out, &out, strings.NewReader(""))
	app.Add(commands...)

	if err := app.Handle(context.Background(), []string{"tags:list", "--tenant=acme"}); console.ExitCode(err) == 0 {
		t.Fatal("a command ran as a subject of another customer")
	}
}

func TestTheCommandsCreateListRenameAndReorder(t *testing.T) {
	t.Parallel()

	service := openService(t, "commands-lifecycle")
	app, out := terminal(t, service)

	if body, code := run(t, app, out, "tags:create", "Draft", "--tenant=acme", "--taxonomy=status"); code != 0 {
		t.Fatalf("creating: exit %d, %s", code, body)
	}
	if body, code := run(t, app, out, "tags:create", "Published", "--tenant=acme", "--taxonomy=status"); code != 0 {
		t.Fatalf("creating: exit %d, %s", code, body)
	}

	body, code := run(t, app, out, "tags:list", "status", "--tenant=acme")
	if code != 0 {
		t.Fatalf("listing: exit %d, %s", code, body)
	}
	if !strings.Contains(body, "draft") || !strings.Contains(body, "published") {
		t.Fatalf("the listing does not show both labels: %s", body)
	}

	held, err := service.List(context.Background(), memberOf("acme"), "status", queryOfEverything())
	if err != nil {
		t.Fatalf("reading them back: %v", err)
	}
	if len(held) != 2 {
		t.Fatalf("the taxonomy holds %d labels", len(held))
	}

	// A rename leaves the slug alone, and the command says so because that is
	// the half somebody is most likely to have expected wrong.
	body, code = run(t, app, out, "tags:rename", held[0].ID, "Draft copy", "--tenant=acme")
	if code != 0 {
		t.Fatalf("renaming: exit %d, %s", code, body)
	}
	if !strings.Contains(body, "draft") {
		t.Fatalf("the rename does not report the slug it kept: %s", body)
	}

	body, code = run(t, app, out, "tags:reorder", "status", "--tenant=acme",
		"--id="+held[1].ID, "--id="+held[0].ID)
	if code != 0 {
		t.Fatalf("reordering: exit %d, %s", code, body)
	}
	after, err := service.List(context.Background(), memberOf("acme"), "status", queryOfEverything())
	if err != nil {
		t.Fatalf("reading the order back: %v", err)
	}
	if after[0].ID != held[1].ID {
		t.Fatalf("the order was not written: %v", names(after))
	}

	body, code = run(t, app, out, "tags:taxonomies", "--tenant=acme")
	if code != 0 || !strings.Contains(body, "status") {
		t.Fatalf("the taxonomies command answered exit %d, %s", code, body)
	}
}

func TestPruneReportsBeforeItDeletes(t *testing.T) {
	t.Parallel()

	service := openService(t, "commands-prune")
	app, out := terminal(t, service)
	actor := memberOf("acme")
	ctx := context.Background()

	carried := mustCreate(t, service, actor, CreateRequest{Name: "Draft", Taxonomy: "status"})
	mustCreate(t, service, actor, CreateRequest{Name: "Orphan", Taxonomy: "status"})
	if err := service.Attach(ctx, actor, mustRef(t, testOwnerKind, "article-1"), carried.ID); err != nil {
		t.Fatalf("attaching: %v", err)
	}

	// A label nothing carries is not the same thing as a label nobody wants, so
	// the default is a report and the flag is where somebody decides.
	body, code := run(t, app, out, "tags:prune", "status", "--tenant=acme")
	if code != 0 {
		t.Fatalf("reporting: exit %d, %s", code, body)
	}
	if !strings.Contains(body, "Orphan") || strings.Contains(body, "deleted") {
		t.Fatalf("the report is wrong or it deleted something: %s", body)
	}
	held, err := service.List(ctx, actor, "status", queryOfEverything())
	if err != nil || len(held) != 2 {
		t.Fatalf("the report wrote something: %v (%v)", names(held), err)
	}

	if body, code := run(t, app, out, "tags:prune", "status", "--tenant=acme", "--apply"); code != 0 {
		t.Fatalf("pruning: exit %d, %s", code, body)
	}
	held, err = service.List(ctx, actor, "status", queryOfEverything())
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if len(held) != 1 || held[0].ID != carried.ID {
		t.Fatalf("after pruning the taxonomy holds %v", names(held))
	}
}

func TestTheShippedPolicyRefusesEveryCommand(t *testing.T) {
	t.Parallel()

	// The service built the way an application builds it, which installs
	// TagPolicy -- and TagPolicy denies everything until somebody opens it. A
	// package whose rules nobody has opened should refuse from a terminal the
	// same way it refuses from a browser.
	service := &TagService{db: openDatabase(t, "commands-shipped-policy"), policy: TagPolicy{}}
	app, out := terminal(t, service)

	for _, args := range [][]string{
		{"tags:list", "--tenant=acme"},
		{"tags:create", "Draft", "--tenant=acme"},
		{"tags:rename", "record-1", "Draft", "--tenant=acme"},
		{"tags:reorder", "--tenant=acme", "--id=record-1"},
		{"tags:taxonomies", "--tenant=acme"},
		{"tags:prune", "--tenant=acme"},
	} {
		body, code := run(t, app, out, args...)
		if code == 0 {
			t.Errorf("%v succeeded against the shipped policy: %s", args, body)
		}
		if !strings.Contains(body, "policy.go") {
			t.Errorf("%v was refused without saying where the rules are: %s", args, body)
		}
	}
}
