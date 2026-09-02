// SMTP configuration: read from the environment, validated at construction.
// The digest is a fire-and-forget background job, so a misconfiguration that
// is not caught here surfaces only as mail that never arrives.
package mail

import "testing"

// TestFromEnv: a misread here silently turns the digest off, or points it at
// the wrong relay, with no error anywhere — the digest simply never arrives.
func TestFromEnv(t *testing.T) {
	clear := func(t *testing.T) {
		t.Helper()
		for _, key := range []string{
			"QUIRE_SMTP_HOST", "QUIRE_SMTP_PORT", "QUIRE_SMTP_USER",
			"QUIRE_SMTP_PASS", "QUIRE_SMTP_FROM",
		} {
			t.Setenv(key, "")
		}
	}

	t.Run("no host means mail is off", func(t *testing.T) {
		clear(t)
		if _, ok := FromEnv(); ok {
			t.Error("mail reported configured with no host")
		}
	})

	t.Run("host alone is enough", func(t *testing.T) {
		clear(t)
		t.Setenv("QUIRE_SMTP_HOST", "smtp.example.com")
		cfg, ok := FromEnv()
		if !ok {
			t.Fatal("mail should be configured")
		}
		if cfg.Port != 587 {
			t.Errorf("default port = %d, want 587", cfg.Port)
		}
	})

	t.Run("from falls back to the user", func(t *testing.T) {
		clear(t)
		t.Setenv("QUIRE_SMTP_HOST", "smtp.example.com")
		t.Setenv("QUIRE_SMTP_USER", "postmaster@example.com")
		cfg, _ := FromEnv()
		if cfg.From != "postmaster@example.com" {
			t.Errorf("From = %q, want the user", cfg.From)
		}
	})

	t.Run("an explicit from wins", func(t *testing.T) {
		clear(t)
		t.Setenv("QUIRE_SMTP_HOST", "smtp.example.com")
		t.Setenv("QUIRE_SMTP_USER", "postmaster@example.com")
		t.Setenv("QUIRE_SMTP_FROM", "quire@example.com")
		cfg, _ := FromEnv()
		if cfg.From != "quire@example.com" {
			t.Errorf("From = %q", cfg.From)
		}
	})

	t.Run("a bad port keeps the default rather than becoming zero", func(t *testing.T) {
		clear(t)
		t.Setenv("QUIRE_SMTP_HOST", "smtp.example.com")
		t.Setenv("QUIRE_SMTP_PORT", "not-a-number")
		cfg, _ := FromEnv()
		if cfg.Port != 587 {
			t.Errorf("port = %d, want the 587 default kept", cfg.Port)
		}
	})

	t.Run("an explicit port is honoured", func(t *testing.T) {
		clear(t)
		t.Setenv("QUIRE_SMTP_HOST", "smtp.example.com")
		t.Setenv("QUIRE_SMTP_PORT", "465")
		cfg, _ := FromEnv()
		if cfg.Port != 465 {
			t.Errorf("port = %d, want 465", cfg.Port)
		}
	})
}

// TestNewSMTPRequiresAddresses: refusing at construction beats failing per
// message at 06:30 with nobody watching the log.
func TestNewSMTPRequiresAddresses(t *testing.T) {
	for _, cfg := range []Config{
		{Host: "", From: "quire@example.com"},
		{Host: "smtp.example.com", From: ""},
		{},
	} {
		if _, err := NewSMTP(cfg); err == nil {
			t.Errorf("NewSMTP(%+v) should have failed", cfg)
		}
	}
	if _, err := NewSMTP(Config{Host: "smtp.example.com", From: "quire@example.com"}); err != nil {
		t.Errorf("a complete config should build: %v", err)
	}
}
