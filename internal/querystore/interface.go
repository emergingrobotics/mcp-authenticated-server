package querystore

import (
	"context"
	"time"
)

// QueryResult is the structured response from a read-only SQL query.
type QueryResult struct {
	Columns   []string        `json:"columns"`
	Rows      [][]interface{} `json:"rows"`
	RowCount  int             `json:"row_count"`
	Truncated bool            `json:"truncated"`
}

// QueryStore executes read-only SQL queries.
type QueryStore interface {
	ExecuteReadOnly(ctx context.Context, query string, params []interface{}, limit int, timeout time.Duration) (*QueryResult, error)
}
