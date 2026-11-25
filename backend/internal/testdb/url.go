// Package testdb selects the database connection for PostgreSQL integration tests.
package testdb

import (
	"os"
	"testing"
)

// URL prefers the suite's connection and falls back to the shared test database.
func URL(t testing.TB, suiteEnv string) string {
	t.Helper()
	for _, name := range []string{suiteEnv, "POSTGRES_TEST_DATABASE_URL"} {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}
	if os.Getenv("REQUIRE_POSTGRES_TESTS") == "1" {
		t.Fatalf("PostgreSQL tests are required: set %s or POSTGRES_TEST_DATABASE_URL", suiteEnv)
	}
	t.Skipf("set %s or POSTGRES_TEST_DATABASE_URL to run the PostgreSQL test", suiteEnv)
	return ""
}
