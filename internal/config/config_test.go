package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(nil, func(string) string { return "" })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.ListenAddr != DefaultListenAddr {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, DefaultListenAddr)
	}
	if cfg.DBPath != DefaultDBPath {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, DefaultDBPath)
	}
	if cfg.BaseURL != DefaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, DefaultBaseURL)
	}
}

func TestLoadEnvironmentFallback(t *testing.T) {
	env := map[string]string{
		EnvListenAddr: "127.0.0.1:9000",
		EnvDBPath:     "/tmp/forum.db",
		EnvBaseURL:    "https://forum.example.com/",
	}

	cfg, err := Load(nil, lookup(env))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.ListenAddr != "127.0.0.1:9000" {
		t.Errorf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.DBPath != "/tmp/forum.db" {
		t.Errorf("DBPath = %q", cfg.DBPath)
	}
	if cfg.BaseURL != "https://forum.example.com" {
		t.Errorf("BaseURL = %q, want trailing slash removed", cfg.BaseURL)
	}
}

func TestFlagsOverrideEnvironment(t *testing.T) {
	env := map[string]string{
		EnvListenAddr: ":9000",
		EnvDBPath:     "/tmp/env.db",
		EnvBaseURL:    "https://env.example.com",
	}

	cfg, err := Load([]string{
		"-listen", ":7000",
		"-db", "/tmp/flag.db",
		"-base-url", "https://flag.example.com",
	}, lookup(env))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.ListenAddr != ":7000" {
		t.Errorf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.DBPath != "/tmp/flag.db" {
		t.Errorf("DBPath = %q", cfg.DBPath)
	}
	if cfg.BaseURL != "https://flag.example.com" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
}

func TestLoadRejectsInvalidBaseURL(t *testing.T) {
	tests := []string{
		"",
		"forum.example.com",
		"ftp://forum.example.com",
		"https://",
	}

	for _, baseURL := range tests {
		t.Run(baseURL, func(t *testing.T) {
			if _, err := Load([]string{"-base-url", baseURL}, func(string) string { return "" }); err == nil {
				t.Fatalf("Load accepted invalid base URL %q", baseURL)
			}
		})
	}
}

func lookup(env map[string]string) func(string) string {
	return func(key string) string { return env[key] }
}

func TestAllowAnyOrigin(t *testing.T) {
	cfg, err := Load(nil, func(string) string { return "" })
	if err != nil || cfg.AllowAnyOrigin {
		t.Fatalf("default: AllowAnyOrigin = %v, err = %v; want false", cfg.AllowAnyOrigin, err)
	}

	cfg, err = Load(nil, lookup(map[string]string{EnvAllowAnyOrigin: "true"}))
	if err != nil || !cfg.AllowAnyOrigin {
		t.Fatalf("env: AllowAnyOrigin = %v, err = %v; want true", cfg.AllowAnyOrigin, err)
	}

	cfg, err = Load([]string{"-allow-any-origin"}, func(string) string { return "" })
	if err != nil || !cfg.AllowAnyOrigin {
		t.Fatalf("flag: AllowAnyOrigin = %v, err = %v; want true", cfg.AllowAnyOrigin, err)
	}

	if _, err := Load(nil, lookup(map[string]string{EnvAllowAnyOrigin: "maybe"})); err == nil {
		t.Fatal("invalid env value: want error")
	}
}
