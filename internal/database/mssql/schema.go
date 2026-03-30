package mssql

import (
	"context"
	"database/sql"
	"fmt"
)

func applySchema(ctx context.Context, db *sql.DB) error {
	// MSSQL uses T-SQL equivalents per MSSQL-02.
	// No vector tables — vector features not available on MSSQL (VEC-02).
	statements := []string{
		`IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'build_metadata')
		CREATE TABLE build_metadata (
			[key] NVARCHAR(255) PRIMARY KEY,
			[value] NVARCHAR(MAX) NOT NULL
		)`,
	}

	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("mssql schema apply: %w\nstatement: %s", err, stmt)
		}
	}

	return nil
}
