package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/conradoqg/statuspage-exporter/internal/collector"
	"github.com/conradoqg/statuspage-exporter/internal/config"
	"github.com/conradoqg/statuspage-exporter/internal/logx"
)

func main() {
	var (
		configPath    string
		listenAddress string
		logLevel      string
	)
	flag.StringVar(&configPath, "config", "config.yaml", "Path to config YAML")
	flag.StringVar(&listenAddress, "listen", ":8080", "Listen address for metrics server")
	flag.StringVar(&logLevel, "log-level", "", "Log level: debug|info|warn|error (overrides config)")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Printf("failed to load config from %s: %v", configPath, err)
		os.Exit(1)
	}

	// CLI flag overrides YAML if provided
	if listenAddress != "" {
		cfg.Server.Listen = listenAddress
	}
	// Configure logging
	if logLevel != "" {
		cfg.Common.LogLevel = logLevel
	}
	logx.SetLevelFromString(cfg.Common.LogLevel)
	logx.Infof("log level set to %s", cfg.Common.LogLevel)

	reg := prometheus.NewRegistry()

	coll, err := collector.New(cfg)
	if err != nil {
		log.Printf("failed to init collector: %v", err)
		os.Exit(1)
	}
	reg.MustRegister(coll)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	srv := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	logx.Infof("statuspage-exporter listening on %s", cfg.Server.Listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logx.Errorf("server error: %v", err)
	}
}
