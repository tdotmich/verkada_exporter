package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/tdotmich/verkada_exporter/internal/collector"
	"github.com/tdotmich/verkada_exporter/internal/verkada"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/exporter-toolkit/web"
)

// version is set at build time by the release pipeline (-ldflags "-X main.version=...").
var version = "dev"

func main() {

	var apiURL string
	var listenAddress string
	var webConfigFilePath string
	var timeout time.Duration
	var showVersion bool

	flag.StringVar(
		&apiURL,
		"verkada.api-url",
		"https://api.verkada.com",
		"Verkada API URL for your region",
	)

	flag.DurationVar(
		&timeout,
		"verkada.timeout",
		20*time.Second,
		"Time limit for the Verkada API calls in one scrape",
	)

	flag.StringVar(
		&listenAddress,
		"web.listen-address",
		":10061",
		"Address for the Prometheus metrics server",
	)

	flag.StringVar(
		&webConfigFilePath,
		"web.config.file",
		"",
		"Optional path to a web config file that turns on TLS or basic auth for /metrics",
	)

	flag.BoolVar(
		&showVersion,
		"version",
		false,
		"Print the version and exit",
	)

	flag.Parse()

	if showVersion {
		fmt.Println("verkada_exporter", version)
		return
	}

	// The API key comes from an environment variable, so it is never in a file or in the process list.
	apiKey, exists := os.LookupEnv("VERKADA_API_KEY")
	if !exists || apiKey == "" {
		log.Fatal("VERKADA_API_KEY is required")
	}

	log.Printf("Starting verkada_exporter %s", version)
	log.Printf("Verkada API URL: %s", apiURL)
	log.Printf("Exporter listening address: %s", listenAddress)

	verkadaClient := verkada.NewClient(apiURL, apiKey, timeout)

	// We use our own registry, so /metrics shows only the metrics we register here.
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collector.New(verkadaClient, timeout),
	)

	landingPage, err := web.NewLandingPage(web.LandingConfig{
		Name:        "Verkada Exporter",
		Description: "Prometheus exporter for Verkada cameras",
		Version:     version,
		Links: []web.LandingLinks{
			{Address: "/metrics", Text: "Metrics"},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.Handle("/", landingPage)

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// The exporter toolkit is the standard Prometheus web server. With -web.config.file
	// it adds TLS and basic auth. Without it, it serves plain HTTP.
	webFlags := &web.FlagConfig{
		WebListenAddresses: &[]string{listenAddress},
		WebSystemdSocket:   new(bool),
		WebConfigFile:      &webConfigFilePath,
	}

	err = web.ListenAndServe(server, webFlags, slog.Default())
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("metrics server: %v", err)
	}
}
