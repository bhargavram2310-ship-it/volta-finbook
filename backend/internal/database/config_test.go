package database

import (
	"strings"
	"testing"
)

func TestParseConfigAcceptsLocalMySQLAndKeepsCredentialsServerSide(t *testing.T) {
	config, err := ParseConfig(`DATABASE_URL="mysql://finbook_app:secret%40value@127.0.0.1:3306/finbook?tls=false"`)
	if err != nil {
		t.Fatal("valid local MySQL URL rejected")
	}
	if config.User != "finbook_app" || config.Passwd != "secret@value" || config.DBName != "finbook" || config.TLSConfig != "" || !config.ParseTime {
		t.Fatal("local MySQL configuration was not parsed correctly")
	}
}

func TestParseConfigRequiresVerifiedTLSForRemoteMySQL(t *testing.T) {
	config, err := ParseConfig("mysql://finbook:secret@db.example.test:3306/finbook?tls=verify-full")
	if err != nil {
		t.Fatal("verified remote MySQL configuration rejected")
	}
	if config.TLSConfig == "" {
		t.Fatal("remote database TLS verification was not configured")
	}

	_, err = ParseConfig("mysql://finbook:secret@db.example.test:3306/finbook?tls=false")
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("remote plaintext MySQL configuration must fail without exposing credentials")
	}
}

func TestParseConfigRejectsPostgreSQLAndInvalidTLSSettings(t *testing.T) {
	for _, value := range []string{
		"postgresql://user:secret@db.example.test/finbook",
		"mysql://user:secret@db.example.test/finbook",
		"mysql://user:secret@127.0.0.1/finbook?tls=skip-verify",
		"mysql://user:@127.0.0.1/finbook?tls=false",
		"mysql://user:secret@127.0.0.1/finbook?tls=false&tls=verify-full",
	} {
		if _, err := ParseConfig(value); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal("invalid database URL was accepted or leaked credentials")
		}
	}
}
