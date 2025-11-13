package providers

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "time"

    "github.com/conradoqg/statuspage-exporter/internal/logx"
)

// Azure DevOps has a public Health API endpoint per docs.
// Example: https://status.dev.azure.com/_apis/status/health?api-version=7.1-preview.1
type AzureDevOpsProvider struct {
    name     string
    apiURL   string
    interval time.Duration
    timeout  time.Duration
    client   *http.Client
}

func NewAzureDevOps(name, baseOrAPI string, interval, timeout time.Duration) *AzureDevOpsProvider {
    u := baseOrAPI
    if u == "" {
        u = "https://status.dev.azure.com/_apis/status/health?api-version=7.1-preview.1"
    } else {
        // If a base like https://status.dev.azure.com is given, attach path
        if parsed, err := url.Parse(u); err == nil && (parsed.Host == "status.dev.azure.com" && parsed.Path == "") {
            u = "https://status.dev.azure.com/_apis/status/health?api-version=7.1-preview.1"
        }
    }
    return &AzureDevOpsProvider{
        name:     name,
        apiURL:   u,
        interval: interval,
        timeout:  timeout,
        client:   NewHTTPClient(timeout),
    }
}

func (p *AzureDevOpsProvider) Interval() time.Duration { return p.interval }
func (p *AzureDevOpsProvider) Timeout() time.Duration  { return p.timeout }

// Minimal shape
type azHealth struct {
    Services []struct {
        Name     string `json:"name"`
        Geography string `json:"geography"`
        Status   string `json:"status"`
    } `json:"services"`
}

func (p *AzureDevOpsProvider) Fetch(ctx context.Context) (Result, error) {
    logx.Debugf("azuredevops fetch url=%s", p.apiURL)
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.apiURL, nil)
    res, err := p.client.Do(req)
    if err != nil {
        return Result{Provider: "azuredevops", Page: p.name}, err
    }
    defer res.Body.Close()
    if res.StatusCode != http.StatusOK {
        return Result{Provider: "azuredevops", Page: p.name}, fmt.Errorf("unexpected status: %s", res.Status)
    }
    var h azHealth
    if err := json.NewDecoder(res.Body).Decode(&h); err != nil {
        return Result{Provider: "azuredevops", Page: p.name}, err
    }
    out := Result{Provider: "azuredevops", Page: p.name}
    for _, s := range h.Services {
        out.Components = append(out.Components, Component{
            Name:   s.Name,
            Region: s.Geography,
            Status: mapAzureDevOps(s.Status),
        })
    }
    logx.Debugf("azuredevops parsed components=%d page=%s", len(out.Components), p.name)
    return out, nil
}

func mapAzureDevOps(s string) NormalizedStatus {
    switch s {
    case "Healthy":
        return StatusOperational
    case "Degraded":
        return StatusDegraded
    case "Unhealthy":
        return StatusMajorOutage
    case "Maintenance":
        return StatusUnderMaintenance
    default:
        return StatusUnknown
    }
}
