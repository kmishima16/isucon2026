package isureal

import "testing"

func TestDatabaseDSNOptions(t *testing.T) {
	base := Config{DBUser: "isureal", DBPassword: "pw", DBHost: "192.168.4.10", DBPort: "5432", DBName: "isureal"}
	if got, want := base.DatabaseDSN(), "postgres://isureal:pw@192.168.4.10:5432/isureal"; got != want {
		t.Fatalf("DSN without options = %q; want %q", got, want)
	}
	far := base
	far.DBExecMode = "exec"
	far.DBSSLMode = "disable"
	if got, want := far.DatabaseDSN(), "postgres://isureal:pw@192.168.4.10:5432/isureal?default_query_exec_mode=exec&sslmode=disable"; got != want {
		t.Fatalf("DSN with options = %q; want %q", got, want)
	}
}
