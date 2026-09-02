package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnrollCodeGatesBootstrap: an unclaimed instance reachable from off-box
// must not hand the vault to whoever arrives first. Before this gate existed,
// POST /api/v1/auth/register/begin returned a valid WebAuthn challenge to any
// anonymous caller, and finishing it minted an owner session.
func TestEnrollCodeGatesBootstrap(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })

	passkeys, err := NewPasskeys(store, "quire", "quire.example.com", []string{"https://quire.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	const code = "ABCDEFGHIJKLMNOP"
	h := &HTTPConfig{Passkeys: passkeys, EnrollCode: code}
	mux := http.NewServeMux()
	h.Routes(mux)

	begin := func(query string) int {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/auth/register/begin"+query, nil))
		return rec.Code
	}

	// No code, wrong code, empty code, and a prefix of the real one: all out.
	for _, query := range []string{
		"",
		"?enroll_code=",
		"?enroll_code=WRONG",
		"?enroll_code=" + code[:len(code)-1],
		"?enroll_code=" + code + "X",
	} {
		if got := begin(query); got != http.StatusUnauthorized {
			t.Errorf("bootstrap with %q = %d, want 401", query, got)
		}
	}
	// The real code works, and is accepted case-insensitively because people
	// retype it off a terminal.
	if got := begin("?enroll_code=" + code); got != http.StatusOK {
		t.Errorf("bootstrap with the real code = %d, want 200", got)
	}
	if got := begin("?enroll_code=" + strings.ToLower(code)); got != http.StatusOK {
		t.Errorf("bootstrap with the lowercased code = %d, want 200", got)
	}

	// Finishing is gated too — checking only the begin leg would let a caller
	// skip straight to the credential-bearing half.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/auth/register/finish?name=x", strings.NewReader("{}")))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("register/finish without a code = %d, want 401", rec.Code)
	}
}

// TestEnrollCodeIsUnguessable: the code is the only thing standing between a
// public URL and vault ownership, so it must carry real entropy and differ
// every time.
func TestEnrollCodeIsUnguessable(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		code, err := NewEnrollCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) < 16 {
			t.Fatalf("enrollment code %q is too short to resist guessing", code)
		}
		if seen[code] {
			t.Fatalf("enrollment code %q was issued twice", code)
		}
		seen[code] = true
	}
}

// TestNoEnrollCodeMeansLoopbackOnly documents the one case the gate is off:
// a loopback listener, where there is no remote caller to race.
func TestNoEnrollCodeMeansLoopbackOnly(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })
	passkeys, err := NewPasskeys(store, "quire", "localhost", []string{"http://localhost:8321"})
	if err != nil {
		t.Fatal(err)
	}
	h := &HTTPConfig{Passkeys: passkeys} // no EnrollCode
	mux := http.NewServeMux()
	h.Routes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/auth/register/begin", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("loopback bootstrap = %d, want 200 (dev must stay frictionless)", rec.Code)
	}
}
