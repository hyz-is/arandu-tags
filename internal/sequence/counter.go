// Package sequence holds the counter that hands out the positions of labels,
// one row per tenant and taxonomy.
//
// It is internal because it is not part of what the tags package offers: it is
// how ordering is claimed, and a caller that wrote to it would be choosing
// positions the claim believes are free. The tags service is its only caller,
// and reaches it after it has authorized, with the Grant that decision
// produced.
package sequence

import (
	"github.com/arandu-io/hesape/database/model"
)

// TableName is the table the counters are stored in.
const TableName = "tag_sequences"

// Counter is the next position to hand out in one taxonomy.
//
// One row per tenant and taxonomy, and its identifier is the pair, so the row
// a claim needs is addressed by key rather than searched for.
type Counter struct {
	model.Model

	// ID is the tenant and the taxonomy, joined.
	ID string `db:"id"`

	// TenantID is the customer the counter belongs to.
	TenantID string `db:"tenant_id"`

	// Type is the taxonomy the counter counts.
	Type string `db:"type"`

	// NextPosition is the position the next claim takes.
	NextPosition int64 `db:"next_position"`
}

// counterTable is the table of Counter. Its query, Counters, is generated
// beside it by aru model:build.
//
// The key is the tenant and the taxonomy, written by the claim, so it does not
// increment. The row carries no time: it is a counter, and when it last moved
// says nothing anybody reads.
var counterTable = model.NewTable(model.TableSpec{
	Name:         TableName,
	New:          func() model.Entity { return new(Counter) },
	ManualKey:    true,
	NoTimestamps: true,
})
