package querystore

import "testing"

func TestValidateQuery_AllowedSelects(t *testing.T) {
	allowed := []string{
		"SELECT * FROM users",
		"SELECT id, name FROM products WHERE price > 10",
		"SELECT COUNT(*) FROM orders",
		"SELECT u.name, o.total FROM users u JOIN orders o ON u.id = o.user_id",
		"SELECT description FROM items", // "description" contains no blocked keyword
	}

	for _, q := range allowed {
		t.Run(q, func(t *testing.T) {
			if err := ValidateQuery(q); err != nil {
				t.Errorf("expected allowed, got: %v", err)
			}
		})
	}
}

func TestValidateQuery_DDLBlocked(t *testing.T) {
	blocked := []string{
		"CREATE TABLE foo (id INT)",
		"ALTER TABLE users ADD COLUMN age INT",
		"DROP TABLE users",
	}
	for _, q := range blocked {
		t.Run(q, func(t *testing.T) {
			if err := ValidateQuery(q); err == nil {
				t.Error("expected blocked DDL")
			}
		})
	}
}

func TestValidateQuery_DMLBlocked(t *testing.T) {
	blocked := []string{
		"INSERT INTO users VALUES (1, 'test')",
		"UPDATE users SET name = 'test'",
		"DELETE FROM users",
		"TRUNCATE TABLE users",
		"MERGE INTO target USING source ON ...",
	}
	for _, q := range blocked {
		t.Run(q, func(t *testing.T) {
			if err := ValidateQuery(q); err == nil {
				t.Error("expected blocked DML")
			}
		})
	}
}

func TestValidateQuery_AdminBlocked(t *testing.T) {
	blocked := []string{
		"GRANT SELECT ON users TO public",
		"REVOKE ALL ON users FROM public",
		"EXEC sp_help",
		"EXECUTE sp_help",
		"xp_cmdshell 'dir'",
		"sp_executesql N'SELECT 1'",
		"SELECT * FROM OPENROWSET('SQLNCLI', ...)",
	}
	for _, q := range blocked {
		t.Run(q, func(t *testing.T) {
			if err := ValidateQuery(q); err == nil {
				t.Error("expected blocked admin command")
			}
		})
	}
}

func TestValidateQuery_TransactionControlBlocked(t *testing.T) {
	blocked := []string{
		"BEGIN TRANSACTION",
		"COMMIT",
		"ROLLBACK",
		"SAVEPOINT sp1",
	}
	for _, q := range blocked {
		t.Run(q, func(t *testing.T) {
			if err := ValidateQuery(q); err == nil {
				t.Error("expected blocked transaction control")
			}
		})
	}
}

func TestValidateQuery_SessionModBlocked(t *testing.T) {
	blocked := []string{
		"SET NOCOUNT ON",
		"DECLARE @x INT",
	}
	for _, q := range blocked {
		t.Run(q, func(t *testing.T) {
			if err := ValidateQuery(q); err == nil {
				t.Error("expected blocked session modification")
			}
		})
	}
}

func TestValidateQuery_SelectIntoBlocked(t *testing.T) {
	if err := ValidateQuery("SELECT * INTO new_table FROM users"); err == nil {
		t.Error("expected SELECT INTO to be blocked")
	}
}

func TestStripComments(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			"SELECT * FROM users -- get all users",
			"SELECT * FROM users ",
		},
		{
			"SELECT * /* this is a block comment */ FROM users",
			"SELECT *  FROM users",
		},
		{
			"SELECT * FROM users -- DROP TABLE users",
			"SELECT * FROM users ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := StripComments(tt.input)
			if got != tt.expected {
				t.Errorf("StripComments(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestStripComments_HiddenKeywords(t *testing.T) {
	// Comments should be stripped before keyword scan
	// This query has DROP hidden in a comment
	query := "SELECT * FROM users -- DROP TABLE users"
	if err := ValidateQuery(query); err != nil {
		t.Errorf("expected allowed (DROP is in comment), got: %v", err)
	}

	query = "SELECT * /* DROP TABLE */ FROM users"
	if err := ValidateQuery(query); err != nil {
		t.Errorf("expected allowed (DROP is in block comment), got: %v", err)
	}
}

func TestHasMultipleStatements(t *testing.T) {
	tests := []struct {
		query    string
		expected bool
	}{
		{"SELECT 1", false},
		{"SELECT 1; SELECT 2", true},
		{"SELECT 1;", true}, // trailing semicolons rejected
		{"SELECT 'a;b' FROM t", false}, // semicolon inside string literal
		{`SELECT "a;b" FROM t`, false}, // semicolon inside double-quoted identifier
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := HasMultipleStatements(tt.query)
			if got != tt.expected {
				t.Errorf("HasMultipleStatements(%q) = %v, want %v", tt.query, got, tt.expected)
			}
		})
	}
}

func TestValidateQuery_CaseInsensitive(t *testing.T) {
	blocked := []string{
		"drop table users",
		"Drop Table Users",
		"INSERT into users values (1)",
		"insert INTO users VALUES (1)",
	}
	for _, q := range blocked {
		t.Run(q, func(t *testing.T) {
			if err := ValidateQuery(q); err == nil {
				t.Errorf("expected blocked (case insensitive): %s", q)
			}
		})
	}
}

func TestValidateQuery_WordBoundaries(t *testing.T) {
	// "DESCRIPTION" should NOT trigger "SET" or any other keyword
	allowed := []string{
		"SELECT description FROM items",
		"SELECT created_at FROM logs",
		"SELECT user_settings FROM config",
		"SELECT resetcount FROM metrics",
	}
	for _, q := range allowed {
		t.Run(q, func(t *testing.T) {
			if err := ValidateQuery(q); err != nil {
				t.Errorf("expected allowed (word boundary), got: %v", err)
			}
		})
	}
}
