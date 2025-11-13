package providers

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "time"

    "github.com/conradoqg/statuspage-exporter/internal/logx"
)

// Google Cloud exposes incidents history at https://status.cloud.google.com/incidents.json
// We infer impacted product/region as degraded/outage if an incident is currently open.
// Otherwise, unknown components are not emitted; this provider focuses on active impacts.
type GCPProvider struct {
    name     string
    url      string
    interval time.Duration
    timeout  time.Duration
    client   *http.Client
}

func NewGCP(name, url string, interval, timeout time.Duration) *GCPProvider {
    if url == "" {
        url = "https://status.cloud.google.com/incidents.json"
    }
    return &GCPProvider{
        name:     name,
        url:      url,
        interval: interval,
        timeout:  timeout,
        client:   NewHTTPClient(timeout),
    }
}

func (p *GCPProvider) Interval() time.Duration { return p.interval }
func (p *GCPProvider) Timeout() time.Duration  { return p.timeout }

func (p *GCPProvider) Fetch(ctx context.Context) (Result, error) {
    logx.Debugf("gcp fetch url=%s", p.url)
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
    res, err := p.client.Do(req)
    if err != nil {
        return Result{Provider: "gcp", Page: p.name}, err
    }
    defer res.Body.Close()
    if res.StatusCode != http.StatusOK {
        return Result{Provider: "gcp", Page: p.name}, fmt.Errorf("unexpected status: %s", res.Status)
    }
    var raw []map[string]any
    if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
        return Result{Provider: "gcp", Page: p.name}, err
    }
    out := Result{Provider: "gcp", Page: p.name}
    open := 0
    for _, inc := range raw {
        // Treat incidents with non-empty end as resolved
        if end, ok := inc["end"].(string); ok && strings.TrimSpace(end) != "" {
            continue
        }
        // Some feeds include a textual status; treat resolved/completed conservatively
        if st, ok := inc["status"].(string); ok {
            s := strings.ToLower(strings.TrimSpace(st))
            if s == "resolved" || s == "completed" {
                continue
            }
        }
        open++
        // Severity mapping from most_recent_update.severity
        var stCode NormalizedStatus = StatusDegraded
        if mru, ok := inc["most_recent_update"].(map[string]any); ok {
            if sev, ok := mru["severity"].(string); ok {
                switch strings.ToLower(sev) {
                case "low":
                    stCode = StatusDegraded
                case "medium":
                    stCode = StatusPartialOutage
                case "high", "critical":
                    stCode = StatusMajorOutage
                default:
                    stCode = StatusDegraded
                }
            }
        }

        // affected_products can be an array of strings or objects with id/title/name
        var prods []string
        if ap, ok := inc["affected_products"].([]any); ok {
            for _, item := range ap {
                switch v := item.(type) {
                case string:
                    prods = append(prods, v)
                case map[string]any:
                    if id, ok := v["id"].(string); ok && id != "" {
                        prods = append(prods, id)
                        continue
                    }
                    if t, ok := v["title"].(string); ok && t != "" {
                        prods = append(prods, t)
                        continue
                    }
                    if n, ok := v["name"].(string); ok && n != "" {
                        prods = append(prods, n)
                        continue
                    }
                }
            }
        }

        // Emit one component per affected product
        for _, prod := range prods {
            out.Components = append(out.Components, Component{
                Name:   prod,
                Status: stCode,
            })
        }
    }
    out.OpenIncidents = open
    logx.Debugf("gcp open_incidents=%d components=%d page=%s", open, len(out.Components), p.name)
    return out, nil
}
