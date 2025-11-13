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
)

func main() {
    var (
        configPath    string
        listenAddress string
    )
    flag.StringVar(&configPath, "config", "config.yaml", "Path to config YAML")
    flag.StringVar(&listenAddress, "listen", ":8080", "Listen address for metrics server")
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

    log.Printf("statuspage-exporter listening on %s", cfg.Server.Listen)
    if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        log.Fatalf("server error: %v", err)
    }
}

