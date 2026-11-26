package alertbox

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/gqlo/alertbox/internal/client"
	"github.com/gqlo/alertbox/internal/server"
	"github.com/gqlo/alertbox/internal/store"
)

// Run starts the Alertbox HTTP server and optional Alertmanager poller.
func Run() {
	st, err := store.NewAlertStore()
	if err != nil {
		log.Fatal(err)
	}

	srv := server.NewServer(st, os.Getenv("SLACK_TOKEN"), os.Getenv("SLACK_CHANNEL_ID"))

	amSession, err := client.NewPollSession(st)
	if err != nil {
		log.Fatal(err)
	}
	if amSession != nil {
		defer amSession.Stop()
		go amSession.Run()
	}

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8083"
	}

	http.HandleFunc("/", srv.HandleAlertsPage)
	http.HandleFunc("/alert", srv.HandleAlertDetail)
	http.HandleFunc("/webhook", srv.HandleWebhook)

	log.Printf("Alertbox listening on %s", addr)
	log.Printf("Dashboard: %s", dashboardURL(addr))
	if srv.SlackEnabled() {
		log.Print("Slack notifications enabled")
	}
	log.Fatal(http.ListenAndServe(addr, nil))
}

func dashboardURL(listenAddr string) string {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return "http://localhost" + listenAddr
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "localhost"
	}
	return fmt.Sprintf("http://%s", net.JoinHostPort(host, port))
}
