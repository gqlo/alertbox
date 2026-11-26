package client

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAlertmanagerURL_default(t *testing.T) {
	unsetEnv(t, "ALERTMANAGER_URL")
	if got := alertmanagerURL(); got != DefaultAlertmanagerURL {
		t.Fatalf("alertmanagerURL() = %q, want %q", got, DefaultAlertmanagerURL)
	}
}

func TestAlertmanagerPollingEnabled(t *testing.T) {
	t.Run("enabled when unset", func(t *testing.T) {
		unsetEnv(t, "ALERTMANAGER_URL")
		if !alertmanagerPollingEnabled() {
			t.Fatal("alertmanagerPollingEnabled() = false, want true when ALERTMANAGER_URL unset")
		}
	})

	t.Run("disabled when empty", func(t *testing.T) {
		t.Setenv("ALERTMANAGER_URL", "")
		if alertmanagerPollingEnabled() {
			t.Fatal("alertmanagerPollingEnabled() = true, want false when ALERTMANAGER_URL=\"\"")
		}
	})

	t.Run("enabled when set", func(t *testing.T) {
		t.Setenv("ALERTMANAGER_URL", "https://alertmanager.example:9094")
		if !alertmanagerPollingEnabled() {
			t.Fatal("alertmanagerPollingEnabled() = false, want true when ALERTMANAGER_URL is set")
		}
	})
}

func TestAlertmanagerTLSInsecure_default(t *testing.T) {
	unsetEnv(t, "ALERTMANAGER_TLS_INSECURE")
	if !alertmanagerTLSInsecure() {
		t.Fatal("alertmanagerTLSInsecure() = false, want true by default")
	}
}

func TestAlertmanagerTLSInsecure_explicit(t *testing.T) {
	t.Setenv("ALERTMANAGER_TLS_INSECURE", "false")
	if alertmanagerTLSInsecure() {
		t.Fatal("alertmanagerTLSInsecure() = true, want false when set to false")
	}
}

func TestAlertmanagerToken_fromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("test-bearer-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unsetEnv(t, "ALERTMANAGER_TOKEN")
	unsetEnv(t, "ALERTMANAGER_TOKEN_FILE")
	t.Setenv("ALERTMANAGER_TOKEN_FILE", path)

	token, source, err := alertmanagerToken()
	if err != nil {
		t.Fatalf("alertmanagerToken() error: %v", err)
	}
	if token != "test-bearer-token" {
		t.Fatalf("token = %q, want %q", token, "test-bearer-token")
	}
	if source != "ALERTMANAGER_TOKEN_FILE" {
		t.Fatalf("source = %q, want ALERTMANAGER_TOKEN_FILE", source)
	}
}

func TestOpenShiftDeployDir_findsRelativePath(t *testing.T) {
	got := openshiftDeployDir()
	if got == "" {
		t.Fatal("openshiftDeployDir() returned empty")
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("openshiftDeployDir() = %q, stat: %v", got, err)
	}
}

func TestAlertmanagerAlertsURL(t *testing.T) {
	got, err := alertmanagerAlertsURL("https://localhost:9094")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://localhost:9094/api/v2/alerts?active=true&inhibited=true&silenced=true&unprocessed=true"
	if got != want {
		t.Fatalf("alertmanagerAlertsURL() = %q, want %q", got, want)
	}
}

func TestIsLocalhostAlertmanagerURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://localhost:9094", true},
		{"https://127.0.0.1:9094", true},
		{"https://alertmanager-main.openshift-monitoring.svc:9094", false},
		{"not-a-url", false},
	}
	for _, tt := range tests {
		if got := isLocalhostAlertmanagerURL(tt.url); got != tt.want {
			t.Errorf("isLocalhostAlertmanagerURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestShouldStartAlertmanagerPortForward(t *testing.T) {
	t.Run("auto when URL unset", func(t *testing.T) {
		unsetEnv(t, "ALERTMANAGER_URL")
		unsetEnv(t, "ALERTMANAGER_PORT_FORWARD")
		if !shouldStartAlertmanagerPortForward() {
			t.Fatal("want auto port-forward when ALERTMANAGER_URL unset")
		}
	})

	t.Run("auto when localhost URL set", func(t *testing.T) {
		t.Setenv("ALERTMANAGER_URL", "https://localhost:9094")
		unsetEnv(t, "ALERTMANAGER_PORT_FORWARD")
		if !shouldStartAlertmanagerPortForward() {
			t.Fatal("want auto port-forward for localhost URL")
		}
	})

	t.Run("skip for in-cluster URL", func(t *testing.T) {
		t.Setenv("ALERTMANAGER_URL", "https://alertmanager-main.openshift-monitoring.svc:9094")
		unsetEnv(t, "ALERTMANAGER_PORT_FORWARD")
		if shouldStartAlertmanagerPortForward() {
			t.Fatal("should not auto port-forward for cluster URL")
		}
	})

	t.Run("disabled explicitly", func(t *testing.T) {
		unsetEnv(t, "ALERTMANAGER_URL")
		t.Setenv("ALERTMANAGER_PORT_FORWARD", "false")
		if shouldStartAlertmanagerPortForward() {
			t.Fatal("ALERTMANAGER_PORT_FORWARD=false should disable")
		}
	})

	t.Run("forced on for localhost", func(t *testing.T) {
		t.Setenv("ALERTMANAGER_URL", "https://localhost:9094")
		t.Setenv("ALERTMANAGER_PORT_FORWARD", "true")
		if !shouldStartAlertmanagerPortForward() {
			t.Fatal("ALERTMANAGER_PORT_FORWARD=true should enable for localhost")
		}
	})

	t.Run("polling disabled", func(t *testing.T) {
		t.Setenv("ALERTMANAGER_URL", "")
		if shouldStartAlertmanagerPortForward() {
			t.Fatal("empty ALERTMANAGER_URL disables polling and port-forward")
		}
	})
}

func TestAlertmanagerPortForwardConfig_defaults(t *testing.T) {
	unsetEnv(t, "ALERTMANAGER_PF_NAMESPACE")
	unsetEnv(t, "ALERTMANAGER_PF_SERVICE")
	unsetEnv(t, "ALERTMANAGER_PF_LOCAL_PORT")
	unsetEnv(t, "ALERTMANAGER_PF_REMOTE_PORT")

	cfg, err := alertmanagerPortForwardConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.namespace != DefaultAlertmanagerPFNamespace || cfg.service != DefaultAlertmanagerPFService {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.localPort != DefaultAlertmanagerPFLocalPort || cfg.remotePort != DefaultAlertmanagerPFRemotePort {
		t.Fatalf("ports = %d/%d", cfg.localPort, cfg.remotePort)
	}
}
