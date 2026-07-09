package main

import "testing"

func TestNormalizeOutboxDBType(t *testing.T) {
	cases := map[string]string{
		"pg":         "postgres",
		"Postgres":   "postgres",
		"POSTGRESQL": "postgres",
		" mysql ":    "mysql", //nolint:gocritic // intentional: asserts the input is trimmed
		"mariadb":    "mysql",
		"mongo":      "mongodb",
		"MongoDB":    "mongodb",
		"oracle":     "",
		"":           "",
	}
	for in, want := range cases {
		if got := normalizeOutboxDBType(in); got != want {
			t.Errorf("normalizeOutboxDBType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMysqlDSN(t *testing.T) {
	// A raw DSN is passed through untouched so callers keep full control.
	raw := "user:pass@tcp(db.internal:3306)/app?tls=true&parseTime=true"
	if got, err := mysqlDSN(raw); err != nil || got != raw {
		t.Fatalf("mysqlDSN(raw) = %q, %v; want unchanged", got, err)
	}

	// A URL is reformatted into a driver DSN.
	got, err := mysqlDSN("mysql://u:p@localhost/app")
	if err != nil {
		t.Fatalf("mysqlDSN(url): %v", err)
	}
	if want := "u:p@tcp(localhost:3306)/app"; got != want {
		t.Errorf("mysqlDSN(url) = %q, want %q", got, want)
	}

	// An explicit port is preserved, and query params carry through.
	got, err = mysqlDSN("mysql://u:p@127.0.0.1:3307/app?parseTime=true")
	if err != nil {
		t.Fatalf("mysqlDSN(url, port): %v", err)
	}
	if want := "u:p@tcp(127.0.0.1:3307)/app?parseTime=true"; got != want {
		t.Errorf("mysqlDSN(url, port) = %q, want %q", got, want)
	}
}
