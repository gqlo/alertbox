package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchAlerts_unauthorized(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	c := &alertmanagerClient{
		baseURL: srv.URL,
		token:   "old-token",
		httpClient: &http.Client{
			Timeout:   5 * time.Second,
			Transport: newAlertmanagerHTTPTransport(true),
		},
	}

	_, err := c.FetchAlerts(context.Background())
	if !isAlertmanagerUnauthorized(err) {
		t.Fatalf("FetchAlerts() error = %v, want unauthorized", err)
	}
}

func TestFetchAlerts_success(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"fingerprint": "abc123",
			"labels":      map[string]string{"alertname": "TestAlert"},
		}})
	}))
	t.Cleanup(srv.Close)

	c := &alertmanagerClient{
		baseURL: srv.URL,
		httpClient: &http.Client{
			Timeout:   5 * time.Second,
			Transport: newAlertmanagerHTTPTransport(true),
		},
	}

	alerts, err := c.FetchAlerts(context.Background())
	if err != nil {
		t.Fatalf("FetchAlerts() error: %v", err)
	}
	if len(alerts) != 1 || alerts[0].Fingerprint != "abc123" {
		t.Fatalf("alerts = %+v", alerts)
	}
}

func TestAlertmanagerClient_refreshTransport(t *testing.T) {
	c := &alertmanagerClient{
		httpClient: &http.Client{
			Timeout:   15 * time.Second,
			Transport: newAlertmanagerHTTPTransport(true),
		},
	}
	oldTransport := c.httpClient.Transport
	c.refreshTransport()
	if c.httpClient.Transport == oldTransport {
		t.Fatal("refreshTransport() did not replace transport")
	}
}

func TestShouldRecoverPortForward(t *testing.T) {
	t.Setenv("ALERTMANAGER_URL", "https://localhost:9094")
	t.Setenv("ALERTMANAGER_PORT_FORWARD", "true")
	if !shouldRecoverPortForward("https://localhost:9094") {
		t.Fatal("want recovery for owned localhost port-forward")
	}

	t.Setenv("ALERTMANAGER_URL", "https://alertmanager-main.openshift-monitoring.svc:9094")
	if shouldRecoverPortForward("https://alertmanager-main.openshift-monitoring.svc:9094") {
		t.Fatal("should not recover port-forward for in-cluster URL")
	}
}
