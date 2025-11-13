package providers

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "time"
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

type gcpIncident struct {
    Number       string   `json:"number"`
    Status       string   `json:"status"`
    ServiceKey   string   `json:"service_key"`
    ExternalDesc string   `json:"external_desc"`
    Affected     []string `json:"affected_products"`
    MostRecentUpdate struct {
        Severity string `json:"severity"`
    } `json:"most_recent_update"`
}

func (p *GCPProvider) Fetch(ctx context.Context) (Result, error) {
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
    res, err := p.client.Do(req)
    if err != nil {
        return Result{Provider: "gcp", Page: p.name}, err
    }
    defer res.Body.Close()
    if res.StatusCode != http.StatusOK {
        return Result{Provider: "gcp", Page: p.name}, fmt.Errorf("unexpected status: %s", res.Status)
    }
    var incidents []gcpIncident
    if err := json.NewDecoder(res.Body).Decode(&incidents); err != nil {
        return Result{Provider: "gcp", Page: p.name}, err
    }
    out := Result{Provider: "gcp", Page: p.name}
    open := 0
    for _, inc := range incidents {
        if strings.ToLower(inc.Status) == "resolved" || strings.ToLower(inc.Status) == "completed" {
            continue
        }
        open++
        sev := strings.ToLower(inc.MostRecentUpdate.Severity)
        var st NormalizedStatus
        switch sev {
        case "low":
            st = StatusDegraded
        case "medium":
            st = StatusPartialOutage
        case "high", "critical":
            st = StatusMajorOutage
        default:
            st = StatusDegraded
        }
        for _, prod := range inc.Affected {
            out.Components = append(out.Components, Component{
                Name:   prod,
                Status: st,
            })
        }
    }
    out.OpenIncidents = open
    return out, nil
}

