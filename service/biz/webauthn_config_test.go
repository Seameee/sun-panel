package biz

import (
	"slices"
	"testing"

	"sun-panel/global"
	"sun-panel/lib/iniConfig"

	"gopkg.in/ini.v1"
)

func TestWebAuthnOriginFromEnv(t *testing.T) {
	tests := []struct {
		input, rpID, origin string
	}{
		{" https://Panel.Example.com/ ", "panel.example.com", "https://panel.example.com"},
		{"https://panel.example.com:8443", "panel.example.com", "https://panel.example.com:8443"},
		{"https://panel.example.com:443", "panel.example.com", "https://panel.example.com"},
		{"http://localhost:3002", "localhost", "http://localhost:3002"},
		{"http://panel.example.com", "", ""},
		{"panel.example.com", "", ""},
		{"https://panel.example.com/dashboard", "", ""},
		{"https://panel.example.com?", "", ""},
		{"https://panel.example.com#", "", ""},
		{"https://panel.example.com:65536", "", ""},
		{"https://panel.example.com:", "", ""},
		{"https://127.0.0.1", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			rpID, origins := webAuthnOriginFromEnv(tc.input)
			var expected []string
			if tc.origin != "" {
				expected = []string{tc.origin}
			}
			if rpID != tc.rpID || !slices.Equal(origins, expected) {
				t.Fatalf("got %q %v, want %q %v", rpID, origins, tc.rpID, expected)
			}
		})
	}
}

func TestWebAuthnConfigEnvOverride(t *testing.T) {
	config, err := ini.Load([]byte(`[security]
webauthn_rp_id=example.com
webauthn_origins=https://panel.example.com,https://panel2.example.com
webauthn_rp_name=Example Panel
`))
	if err != nil {
		t.Fatal(err)
	}
	old := global.Config
	t.Cleanup(func() { global.Config = old })
	global.Config = &iniConfig.IniConfig{Config: config}

	t.Setenv("SUN_PANEL_WEBAUTHN_ORIGIN", "")
	cfg := Security.Config()
	if cfg.RPID != "example.com" || len(cfg.RPOrigins) != 2 || !cfg.PasskeyAvailable() {
		t.Fatalf("existing INI config = %#v", cfg)
	}
	t.Setenv("SUN_PANEL_WEBAUTHN_ORIGIN", "https://other.example.net:8443")
	cfg = Security.Config()
	if cfg.RPID != "other.example.net" || !slices.Equal(cfg.RPOrigins, []string{"https://other.example.net:8443"}) || cfg.RPName != "Example Panel" {
		t.Fatalf("environment override = %#v", cfg)
	}
	t.Setenv("SUN_PANEL_WEBAUTHN_ORIGIN", "http://other.example.net")
	if cfg = Security.Config(); cfg.PasskeyAvailable() || cfg.RPID != "" || len(cfg.RPOrigins) != 0 {
		t.Fatalf("invalid environment override = %#v", cfg)
	}
	global.Config = nil
	t.Setenv("SUN_PANEL_WEBAUTHN_ORIGIN", "https://other.example.net")
	if cfg = Security.Config(); !cfg.PasskeyAvailable() || cfg.RPID != "other.example.net" {
		t.Fatalf("environment-only config = %#v", cfg)
	}
}
