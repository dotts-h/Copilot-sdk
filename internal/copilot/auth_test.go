package copilot

import (
	"context"
	"errors"
	"testing"

	sdk "github.com/github/copilot-sdk/go"
)

// ResolveAuthMethod dispatches the configured auth method (ADR-0039) onto the
// SDK options pair: an explicit token wins, "gh" resolves via the gh CLI seam,
// and anything that can't produce a token degrades to the logged-in CLI session
// (auto) rather than failing the dial.
func TestResolveAuthMethod(t *testing.T) {
	ghOK := func() (string, error) { return "gho_tok", nil }
	ghEmpty := func() (string, error) { return "", nil }
	ghErr := func() (string, error) { return "", errors.New("gh: not logged in") }

	cases := []struct {
		name       string
		method     string
		configured string
		gh         func() (string, error)
		wantToken  string
		wantAuto   bool // UseLoggedInUser == &true (no explicit token)
	}{
		{"auto no token", "", "", nil, "", true},
		{"auto with configured token", "", "ghp_cfg", nil, "ghp_cfg", false},
		{"token method", "token", "ghp_cfg", nil, "ghp_cfg", false},
		{"token method, var unset, degrades to auto", "token", "", nil, "", true},
		{"gh method resolves via gh CLI", "gh", "ghp_cfg", ghOK, "gho_tok", false},
		{"gh method, gh errors, degrades to auto", "gh", "", ghErr, "", true},
		{"gh method, empty token, degrades to auto", "gh", "", ghEmpty, "", true},
		{"gh method, nil seam, degrades to auto", "gh", "", nil, "", true},
		{"unknown method falls through to the auto path (configured token wins)", "bogus", "ghp_cfg", nil, "ghp_cfg", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token, loggedIn := ResolveAuthMethod(tc.method, tc.configured, tc.gh)
			if token != tc.wantToken {
				t.Errorf("token = %q, want %q", token, tc.wantToken)
			}
			if tc.wantAuto {
				if loggedIn == nil || !*loggedIn {
					t.Errorf("UseLoggedInUser = %v, want &true (auto)", loggedIn)
				}
			} else if loggedIn != nil {
				t.Errorf("UseLoggedInUser = %v, want nil for an explicit token", *loggedIn)
			}
		})
	}
}

// MockClient carries a settable canned AuthStatus so UI tests drive the
// Connection page without a runtime (seam-purity rule).
func TestMockClientAuthStatus(t *testing.T) {
	m := NewMockClient()

	// Default: an honest offline status, not a fake login.
	st, err := m.AuthStatus(context.Background())
	if err != nil {
		t.Fatalf("default AuthStatus: %v", err)
	}
	if st.Authenticated || st.Method == "" {
		t.Errorf("default mock status should be unauthenticated with a method label, got %+v", st)
	}

	m.Auth = AuthStatus{Authenticated: true, Method: "oauth", Login: "octocat", Host: "github.com"}
	st, err = m.AuthStatus(context.Background())
	if err != nil || !st.Authenticated || st.Login != "octocat" {
		t.Errorf("settable status not returned: %+v err=%v", st, err)
	}

	m.AuthErr = errors.New("boom")
	if _, err := m.AuthStatus(context.Background()); err == nil {
		t.Error("AuthErr not surfaced")
	}
}

// authStatusFromSDK normalizes the SDK response, nil-safe on every pointer
// field (the AuthType vocabulary is opaque display text — spike 0067).
func TestAuthStatusFromSDK(t *testing.T) {
	if st := authStatusFromSDK(nil); st != (AuthStatus{}) {
		t.Errorf("nil response should map to the zero status, got %+v", st)
	}

	bare := &sdk.GetAuthStatusResponse{IsAuthenticated: false}
	if st := authStatusFromSDK(bare); st.Authenticated || st.Method != "" || st.Login != "" {
		t.Errorf("bare response should map to an empty unauthenticated status, got %+v", st)
	}

	authType, host, login, msg := "oauth", "github.com", "octocat", "ok"
	full := &sdk.GetAuthStatusResponse{
		IsAuthenticated: true, AuthType: &authType, Host: &host, Login: &login, StatusMessage: &msg,
	}
	st := authStatusFromSDK(full)
	if !st.Authenticated || st.Method != "oauth" || st.Host != "github.com" ||
		st.Login != "octocat" || st.Detail != "ok" {
		t.Errorf("full response mis-mapped: %+v", st)
	}
}
