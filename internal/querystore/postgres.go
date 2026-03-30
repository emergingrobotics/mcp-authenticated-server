package querystore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// PostgresQueryStore executes read-only queries on PostgreSQL.
type PostgresQueryStore struct {
	db              *sql.DB
	maxResponseSize int64
}

// NewPostgresQueryStore creates a PostgreSQL query store.
func NewPostgresQueryStore(db *sql.DB, maxResponseSize int64) *PostgresQueryStore {
	return &PostgresQueryStore{db: db, maxResponseSize: maxResponseSize}
}

func (s *PostgresQueryStore) ExecuteReadOnly(ctx context.Context, query string, params []interface{}, limit int, timeout time.Duration) (*QueryResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("beginning read-only transaction: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, query, params...)
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

		// Convert byte slices to strings for JSON compatibility
		for i, v := range values {
			if b, ok := v.([]byte); ok {
				values[i] = string(b)
			}
		}

		// Check response size
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
	return result, tx.Commit()
}

func categorizeError(err error) error {
	if err == nil {
		return nil
	}
	errStr := err.Error()
	// Syntax errors are safe to return (SQL-07)
	if containsSyntaxError(errStr) {
		return err
	}
	// Generic message for other errors (SQL-07)
	return fmt.Errorf("query execution failed")
}

func containsSyntaxError(errStr string) bool {
	for _, pattern := range []string{"syntax error", "SYNTAX ERROR"} {
		if len(errStr) >= len(pattern) {
			for i := 0; i <= len(errStr)-len(pattern); i++ {
				match := true
				for j := range pattern {
					if (errStr[i+j] | 0x20) != (pattern[j] | 0x20) {
						match = false
						break
					}
				}
				if match {
					return true
				}
			}
		}
	}
	return false
}
