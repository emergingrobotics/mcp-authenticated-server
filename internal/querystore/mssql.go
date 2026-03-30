package querystore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// MSSQLQueryStore executes read-only queries on MS SQL Server.
// Write prevention relies on read-only DB user (DB-09) + keyword blocking (SQL-06).
type MSSQLQueryStore struct {
	db              *sql.DB
	maxResponseSize int64
}

// NewMSSQLQueryStore creates an MSSQL query store.
func NewMSSQLQueryStore(db *sql.DB, maxResponseSize int64) *MSSQLQueryStore {
	return &MSSQLQueryStore{db: db, maxResponseSize: maxResponseSize}
}

func (s *MSSQLQueryStore) ExecuteReadOnly(ctx context.Context, query string, params []interface{}, limit int, timeout time.Duration) (*QueryResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// MSSQL uses read-only DB user as primary control (DB-09).
	// No SET TRANSACTION READ ONLY equivalent needed.
	rows, err := s.db.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, categorizeError(err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("getting columns: %w", err)
	}

	result := &QueryResult{
		Columns: columns,
	}

	var totalSize int64
	rowCount := 0

	for rows.Next() {
		if rowCount >= limit {
			result.Truncated = true
			break
		}

		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}

		for i, v := range values {
			if b, ok := v.([]byte); ok {
				values[i] = string(b)
			}
		}

		rowJSON, _ := json.Marshal(values)
		totalSize += int64(len(rowJSON))
		if totalSize > s.maxResponseSize {
			result.Truncated = true
			break
		}

		result.Rows = append(result.Rows, values)
		rowCount++
	}

	if err := rows.Err(); err != nil {
		return nil, categorizeError(err)
	}

	result.RowCount = len(result.Rows)
	return result, nil
}
