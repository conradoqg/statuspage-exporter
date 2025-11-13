package providers

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

// Status.io requires the per-page Public Status API endpoint (unique URL).
// Example response shape documented by status.io KB.
type StatusIOProvider struct {
    name     string
    apiURL   string
    interval time.Duration
    timeout  time.Duration
    client   *http.Client
}

func NewStatusIO(name, apiURL string, interval, timeout time.Duration) *StatusIOProvider {
    return &StatusIOProvider{
        name:     name,
        apiURL:   apiURL,
        interval: interval,
        timeout:  timeout,
        client:   NewHTTPClient(timeout),
    }
}

func (p *StatusIOProvider) Interval() time.Duration { return p.interval }
func (p *StatusIOProvider) Timeout() time.Duration  { return p.timeout }

// Minimal subset of fields we need
type statusioResp struct {
    Result struct {
        Status []struct {
            Name       string `json:"name"`
            Status     string `json:"status"`
            StatusCode int    `json:"status_code"`
        } `json:"status"`
    } `json:"result"`
}

func (p *StatusIOProvider) Fetch(ctx context.Context) (Result, error) {
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.apiURL, nil)
    res, err := p.client.Do(req)
    if err != nil {
        return Result{Provider: "statusio", Page: p.name}, err
    }
    defer res.Body.Close()
    if res.StatusCode != http.StatusOK {
        return Result{Provider: "statusio", Page: p.name}, fmt.Errorf("unexpected status: %s", res.Status)
    }
    var r statusioResp
    if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
        return Result{Provider: "statusio", Page: p.name}, err
    }
    out := Result{Provider: "statusio", Page: p.name}
    for _, s := range r.Result.Status {
        out.Components = append(out.Components, Component{
            Name:   s.Name,
            Status: mapStatusIOCode(s.StatusCode),
        })
    }
    return out, nil
}

// Status.io status codes reference: 100 operational, 200 maintenance, 300 degraded performance,
// 400 partial service disruption, 500 service disruption, 600 security event
func mapStatusIOCode(code int) NormalizedStatus {
    switch code {
    case 100:
        return StatusOperational
    case 200:
        return StatusUnderMaintenance
    case 300:
        return StatusDegraded
    case 400:
        return StatusPartialOutage
    case 500, 600:
        return StatusMajorOutage
    default:
        return StatusUnknown
    }
}

