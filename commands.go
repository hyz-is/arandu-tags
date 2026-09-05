// What an operator runs from a terminal.
//
// They are values of the framework's own command type, built by a constructor
// this package exports, and the application adds them to its console beside
// every other group it holds. They are not a program: a package that carried a
// main would be a second way to reach these tables, compiled by nobody who
// installed it and audited by nothing -- see PublishCommand for the same
// reasoning on the other half of the install.
//
// # Every command authorizes, and none of them invents a subject
//
// A command reaches the same service the routes do, so it passes the same
// policy. Who it runs as is the application's answer and not this package's:
// Deps carries an Operator, the application writes it, and a package that
// minted a subject for itself would be a package that authorizes itself. With
// the policy this package ships, every command below is refused -- which is
// what a package whose rules nobody has opened yet should do from a terminal as
// well as from a browser.

package tags

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/console"
)

// CommandPrefix is what every command of this package is called under, so
// `aru list` groups them and two packages cannot claim one name.
const CommandPrefix = "tags:"

// Deps is what the commands need, built by the application where it wires
// everything else.
//
// It is a struct and not a list of parameters because the set grows: a
// constructor per command, each threading the same two values, is the same
// wiring written six times and corrected in five of them.
type Deps struct {
	// Service is the same service the routes call. One of it, holding one
	// database handle, so a label written from a terminal and a label read
	// through a request are the same rows decided by the same policy.
	Service *TagService

	// Operator says who a command runs as, for the customer it names.
	//
	// It is the application's decision. A command has no session to read a
	// subject from, and a subject this package invented would be one no policy
	// of the application ever agreed to -- so the application writes the
	// function, and what it returns is what the policy is asked about.
	Operator func(tenant string) security.Subject
}

// Validate reports what the dependencies cannot be used with.
func (d Deps) Validate() error {
	if d.Service == nil {
		return errors.New("tags: Deps.Service is required: a command with no service has no policy to pass and no rows to read")
	}
	if d.Operator == nil {
		return errors.New("tags: Deps.Operator is required: a command runs as somebody, and who that is belongs to the application rather than to this package")
	}
	return nil
}

// Commands builds every command of this package against one set of
// dependencies, so an application registers the group in a single call rather
// than naming each command and threading the same values through all of them.
//
// It returns an error rather than panicking, for the reason New does: everything
// it refuses is a wiring mistake, and a wiring mistake found where the console
// is assembled costs one restart.
func Commands(deps Deps) ([]console.Command, error) {
	if err := deps.Validate(); err != nil {
		return nil, err
	}
	return []console.Command{
		listCommand(deps),
		createCommand(deps),
		renameCommand(deps),
		reorderCommand(deps),
		taxonomiesCommand(deps),
		pruneCommand(deps),
	}, nil
}

// listCommand prints a taxonomy in the order it is read in.
func listCommand(deps Deps) console.Command {
	return console.Command{
		Signature: CommandPrefix + "list {taxonomy? : The taxonomy to list, empty for the one with no name}" +
			" {--tenant= : The customer to read as}" +
			" {--search= : Narrow to the labels whose name contains this}",
		Description: "list the labels of a taxonomy",
		Run: func(ctx context.Context, o *console.IO) error {
			actor, tenant, err := operator(deps, o)
			if err != nil {
				return err
			}
			taxonomy := o.Argument("taxonomy").String()

			var records []*Tag
			if search := o.Option("search").String(); search != "" {
				records, err = deps.Service.Search(ctx, actor, taxonomy, search, data.Query{Limit: MaxIDsPerQuery})
			} else {
				records, err = deps.Service.List(ctx, actor, taxonomy, data.Query{Limit: MaxIDsPerQuery})
			}
			if err != nil {
				return fail(o, err)
			}
			if len(records) == 0 {
				o.Comment("%s holds no label in %s", tenant, quoted(taxonomy))
				return nil
			}

			rows := make([][]string, 0, len(records))
			for _, record := range records {
				rows = append(rows, []string{
					record.ID, record.Name, record.Slug, strconv.FormatInt(record.Position, 10),
				})
			}
			o.Table([]string{"id", "name", "slug", "position"}, rows)
			return nil
		},
	}
}

// createCommand adds a label to a taxonomy.
func createCommand(deps Deps) console.Command {
	return console.Command{
		Signature: CommandPrefix + "create {name : What the label will be called}" +
			" {--tenant= : The customer to write for}" +
			" {--taxonomy= : The taxonomy it joins, empty for the one with no name}",
		Description: "create a label in a taxonomy",
		Run: func(ctx context.Context, o *console.IO) error {
			actor, _, err := operator(deps, o)
			if err != nil {
				return err
			}

			record, err := deps.Service.Create(ctx, actor, CreateRequest{
				Name:     o.Argument("name").String(),
				Taxonomy: o.Option("taxonomy").String(),
			})
			if err != nil {
				return fail(o, err)
			}
			o.Info("%s created as %s at position %d", record.Slug, record.ID, record.Position)
			return nil
		},
	}
}

// renameCommand changes what a label is called.
func renameCommand(deps Deps) console.Command {
	return console.Command{
		Signature: CommandPrefix + "rename {id : The identifier of the label}" +
			" {name : What it will be called from now on}" +
			" {--tenant= : The customer it belongs to}",
		Description: "rename a label, leaving its slug alone",
		Run: func(ctx context.Context, o *console.IO) error {
			actor, _, err := operator(deps, o)
			if err != nil {
				return err
			}

			record, err := deps.Service.Rename(ctx, actor, o.Argument("id").String(), RenameRequest{
				Name: o.Argument("name").String(),
			})
			if err != nil {
				return fail(o, err)
			}
			// The slug is printed because it did not change, which is the half
			// of this command somebody is most likely to have expected wrong.
			o.Info("%s is now %q, and its slug is still %s", record.ID, record.Name, record.Slug)
			return nil
		},
	}
}

// reorderCommand writes one order over a whole taxonomy.
func reorderCommand(deps Deps) console.Command {
	return console.Command{
		Signature: CommandPrefix + "reorder {taxonomy? : The taxonomy to rearrange, empty for the one with no name}" +
			" {--tenant= : The customer it belongs to}" +
			" {--id=* : The identifiers, in the order they are to take}",
		Description: "write one order over a whole taxonomy",
		Run: func(ctx context.Context, o *console.IO) error {
			actor, _, err := operator(deps, o)
			if err != nil {
				return err
			}

			records, err := deps.Service.Reorder(ctx, actor, o.Argument("taxonomy").String(), o.Option("id").Slice())
			if err != nil {
				return fail(o, err)
			}
			for _, record := range records {
				o.TwoColumnDetail(record.Name, strconv.FormatInt(record.Position, 10))
			}
			o.Info("%d labels reordered", len(records))
			return nil
		},
	}
}

// taxonomiesCommand prints the taxonomies a customer holds labels in.
func taxonomiesCommand(deps Deps) console.Command {
	return console.Command{
		Signature:   CommandPrefix + "taxonomies {--tenant= : The customer to read as}",
		Description: "list the taxonomies a customer holds labels in",
		Run: func(ctx context.Context, o *console.IO) error {
			actor, tenant, err := operator(deps, o)
			if err != nil {
				return err
			}

			found, err := deps.Service.Taxonomies(ctx, actor)
			if err != nil {
				return fail(o, err)
			}
			if len(found) == 0 {
				o.Comment("%s holds no label at all", tenant)
				return nil
			}
			for _, taxonomy := range found {
				o.Line("%s", quoted(taxonomy))
			}
			return nil
		},
	}
}

// pruneCommand reports the labels nothing carries, and deletes them when told
// to.
//
// It reports by default and writes only with --apply, because a label nothing
// carries is not the same thing as a label nobody wants: one created for next
// month is carried by nothing today. The list is what an operator reads before
// deciding, and the flag is where they decide.
func pruneCommand(deps Deps) console.Command {
	return console.Command{
		Signature: CommandPrefix + "prune {taxonomy? : The taxonomy to sweep, empty for the one with no name}" +
			" {--tenant= : The customer it belongs to}" +
			" {--apply : Delete them, instead of reporting them}",
		Description: "report the labels nothing carries, and delete them with --apply",
		Run: func(ctx context.Context, o *console.IO) error {
			actor, tenant, err := operator(deps, o)
			if err != nil {
				return err
			}
			taxonomy := o.Argument("taxonomy").String()

			unused, err := deps.Service.UnusedTags(ctx, actor, taxonomy)
			if err != nil {
				return fail(o, err)
			}
			if len(unused) == 0 {
				o.Comment("every label of %s in %s is carried by something", quoted(taxonomy), tenant)
				return nil
			}

			for _, record := range unused {
				o.TwoColumnDetail(record.Name, record.ID)
			}
			if !o.Option("apply").Bool() {
				o.Comment("%d labels are carried by nothing. Run again with --apply to delete them", len(unused))
				return nil
			}
			for _, record := range unused {
				if err := deps.Service.Delete(ctx, actor, record.ID); err != nil {
					return fail(o, err)
				}
			}
			o.Info("%d labels deleted", len(unused))
			return nil
		},
	}
}

// operator reads the customer off the command line and asks the application who
// the command runs as.
//
// The tenant is a flag and not a value this package works out, and it is the one
// place a tenant is named from outside a Grant. It is not the same thing as
// reading one out of a request: a request comes from whoever is on the far end
// of the internet, and this comes from whoever holds the terminal the process
// runs on -- the person who could read the database directly anyway.
func operator(deps Deps, o *console.IO) (security.Subject, string, error) {
	tenant := strings.TrimSpace(o.Option("tenant").String())
	if tenant == "" {
		return security.Subject{}, "", console.Exit(1, "--tenant is required: a command reads one customer's labels, and there is no session here to say which")
	}
	if !security.ValidTenant(tenant) {
		return security.Subject{}, "", console.Exit(1, "--tenant is %q, which cannot be a tenant: lowercase letters, digits, - and _, up to 64 characters", tenant)
	}

	actor := deps.Operator(tenant)
	if actor.Tenant != tenant {
		// The application's own function is what decides who runs; this only
		// refuses the one answer that cannot be right. A subject of another
		// customer would read that customer's rows under a command line that
		// named this one, and the operator would never see it.
		return security.Subject{}, "", console.Exit(1,
			"the operator for %q belongs to %q, so the command would read another customer's labels", tenant, actor.Tenant)
	}
	return actor, tenant, nil
}

// fail turns what the service refused into an exit code and one line.
//
// A refusal is a 1 and a lost race is a 75, which is what an operator's shell
// wrapper reads to decide whether running it again is worth anything: a policy
// that said no says no again, and a position that could not be claimed is free
// by the time the next attempt reads it.
func fail(o *console.IO, err error) error {
	switch {
	case errors.Is(err, security.ErrForbidden):
		return console.Exit(1, "the policy refused this. Open the action in policy.go")
	case errors.Is(err, ErrNotFound):
		return console.Exit(1, "there is no such label")
	case errors.Is(err, ErrSlugTaken):
		return console.Exit(1, "%v", err)
	case errors.Is(err, ErrOrderIncomplete):
		return console.Exit(1, "%v", err)
	case errors.Is(err, ErrPositionUnavailable), errors.Is(err, ErrOrderChanged):
		return console.Exit(75, "%v", err)
	}
	return console.Exit(1, "%v", err)
}

// quoted is a taxonomy as a line of output names it, so the one with no name is
// visible rather than an empty gap.
func quoted(taxonomy string) string {
	if taxonomy == DefaultTaxonomy {
		return "the taxonomy with no name"
	}
	return fmt.Sprintf("%q", taxonomy)
}
