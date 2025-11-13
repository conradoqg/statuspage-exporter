package providers

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "time"
)

// Instatus pages expose components at /v2/components.json (preferred) and legacy /summary.json
type InstatusProvider struct {
    name     string
    baseURL  string
    interval time.Duration
    timeout  time.Duration
    client   *http.Client
}

func NewInstatus(name, baseURL string, interval, timeout time.Duration) *InstatusProvider {
    if !strings.HasPrefix(baseURL, "http") {
        baseURL = "https://" + baseURL
    }
    return &InstatusProvider{
        name:     name,
        baseURL:  strings.TrimRight(baseURL, "/"),
        interval: interval,
        timeout:  timeout,
        client:   NewHTTPClient(timeout),
    }
}

func (p *InstatusProvider) Interval() time.Duration { return p.interval }
func (p *InstatusProvider) Timeout() time.Duration  { return p.timeout }

type instatusComponent struct {
    Name   string `json:"name"`
    Status string `json:"status"`
    Group  string `json:"group_name"`
}

type instatusV2 struct {
    Components []instatusComponent `json:"components"`
}

func (p *InstatusProvider) Fetch(ctx context.Context) (Result, error) {
    // Try v2 first
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/v2/components.json", nil)
    res, err := p.client.Do(req)
    if err != nil {
        return Result{Provider: "instatus", Page: p.name}, err
    }
    defer res.Body.Close()
    if res.StatusCode == http.StatusNotFound {
        // fallback to legacy summary
        return p.fetchLegacy(ctx)
    }
    if res.StatusCode != http.StatusOK {
        return Result{Provider: "instatus", Page: p.name}, fmt.Errorf("unexpected status: %s", res.Status)
    }
    var v instatusV2
    if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
        return Result{Provider: "instatus", Page: p.name}, err
    }
    out := Result{Provider: "instatus", Page: p.name}
    for _, c := range v.Components {
        out.Components = append(out.Components, Component{
            Name:   c.Name,
            Group:  c.Group,
            Status: mapInstatus(c.Status),
        })
    }
    return out, nil
}

func (p *InstatusProvider) fetchLegacy(ctx context.Context) (Result, error) {
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/summary.json", nil)
    res, err := p.client.Do(req)
    if err != nil {
        return Result{Provider: "instatus", Page: p.name}, err
    }
    defer res.Body.Close()
    if res.StatusCode != http.StatusOK {
        return Result{Provider: "instatus", Page: p.name}, fmt.Errorf("unexpected status: %s", res.Status)
    }
    var obj map[string]any
    if err := json.NewDecoder(res.Body).Decode(&obj); err != nil {
        return Result{Provider: "instatus", Page: p.name}, err
    }
    out := Result{Provider: "instatus", Page: p.name}
    // Best-effort parse: expect components under "components"
    if comps, ok := obj["components"].([]any); ok {
        for _, raw := range comps {
            m, _ := raw.(map[string]any)
            name, _ := m["name"].(string)
            status, _ := m["status"].(string)
            group, _ := m["group_name"].(string)
            out.Components = append(out.Components, Component{
                Name:   name,
                Group:  group,
                Status: mapInstatus(status),
            })
        }
    }
    return out, nil
}

func mapInstatus(s string) NormalizedStatus {
    switch strings.ToUpper(s) {
    case "OPERATIONAL":
        return StatusOperational
    case "UNDERMAINTENANCE", "MAINTENANCE":
        return StatusUnderMaintenance
    case "DEGRADEDPERFORMANCE", "DEGRADED":
        return StatusDegraded
    case "PARTIALOUTAGE":
        return StatusPartialOutage
    case "MAJOROUTAGE", "OUTAGE":
        return StatusMajorOutage
    default:
        return StatusUnknown
    }
}

