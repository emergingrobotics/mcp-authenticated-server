package querystore

import (
	"fmt"
	"regexp"
	"strings"
)

// blockedKeywords are SQL keywords that are not allowed in read-only queries (SQL-06).
var blockedKeywords = []string{
	// DDL
	"CREATE", "ALTER", "DROP",
	// DML mutations
	"INSERT", "UPDATE", "DELETE", "TRUNCATE", "MERGE", "REPLACE",
	// Administrative
	"GRANT", "REVOKE", "EXEC", "EXECUTE",
	"OPENROWSET", "OPENDATASOURCE", "BULK", "COPY", "LOAD", "CALL",
	// Transaction control
	"BEGIN", "COMMIT", "ROLLBACK", "SAVEPOINT",
	// Session modification
	"SET", "DECLARE",
}

// blockedPrefixes are prefix patterns to block (e.g., xp_, sp_executesql).
var blockedPrefixes = []string{"xp_", "sp_executesql"}

// wordBoundaryPattern builds a regex that matches a keyword at word boundaries.
func wordBoundaryPattern(keyword string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(keyword) + `\b`)
}

// selectIntoPattern matches SELECT ... INTO (SQL-06).
var selectIntoPattern = regexp.MustCompile(`(?i)\bSELECT\b[^;]*\bINTO\b`)

// ValidateQuery checks a SQL query for disallowed operations.
func ValidateQuery(query string) error {
	// Strip comments before scanning
	stripped := StripComments(query)

	// Check for multiple statements (semicolons outside string literals)
	if HasMultipleStatements(stripped) {
		return fmt.Errorf("query contains disallowed keywords: multiple statements not allowed")
	}

	// Check for blocked keywords
	if keyword, found := ContainsBlockedKeyword(stripped); found {
		return fmt.Errorf("query contains disallowed keywords: %s", keyword)
	}

	return nil
}

// StripComments removes SQL comments from a query.
func StripComments(query string) string {
	// Remove line comments (-- to end of line)
	lineComment := regexp.MustCompile(`--[^\n]*`)
	result := lineComment.ReplaceAllString(query, "")

	// Remove block comments (/* ... */)
	blockComment := regexp.MustCompile(`/\*[\s\S]*?\*/`)
	result = blockComment.ReplaceAllString(result, "")

	return result
}

// ContainsBlockedKeyword checks for blocked SQL keywords at word boundaries.
// Returns the matched keyword and true if found.
func ContainsBlockedKeyword(query string) (string, bool) {
	upper := strings.ToUpper(query)

	// Check SELECT INTO
	if selectIntoPattern.MatchString(query) {
		return "SELECT INTO", true
	}

	// Check blocked keywords at word boundaries
	for _, kw := range blockedKeywords {
		pattern := wordBoundaryPattern(kw)
		if pattern.MatchString(query) {
			// Avoid false positives: "DESCRIPTION" should not match "SET"
			_ = upper // used implicitly via case-insensitive regex
			return kw, true
		}
	}

	// Check blocked prefixes
	for _, prefix := range blockedPrefixes {
		pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(prefix))
		if pattern.MatchString(query) {
			return prefix, true
		}
	}

	return "", false
}

// HasMultipleStatements checks for semicolons outside string literals.
func HasMultipleStatements(query string) bool {
	inSingleQuote := false
	inDoubleQuote := false

	for i := 0; i < len(query); i++ {
		ch := query[i]

		switch {
		case ch == '\'' && !inDoubleQuote:
			// Handle escaped single quotes
			if i+1 < len(query) && query[i+1] == '\'' {
				i++ // skip escaped quote
			} else {
				inSingleQuote = !inSingleQuote
			}
		case ch == '"' && !inSingleQuote:
			inDoubleQuote = !inDoubleQuote
		case ch == ';' && !inSingleQuote && !inDoubleQuote:
			// Check if there's meaningful content after the semicolon
			remaining := strings.TrimSpace(query[i+1:])
			if remaining != "" {
				return true
			}
			// Trailing semicolons with no content after are also rejected for safety
			return true
		}
	}

	return false
}
