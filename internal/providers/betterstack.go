package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Better Stack public pages do not expose a documented unauthenticated summary endpoint.
// We support their REST API with a Read token and Status Page ID.
// API docs: https://betterstack.com/docs/uptime/api/ (requires token)
type BetterStackProvider struct {
	name       string
	pageID     string
	apiToken   string
	interval   time.Duration
	timeout    time.Duration
	httpClient *http.Client
}

func NewBetterStack(name, pageID, apiToken string, interval, timeout time.Duration) *BetterStackProvider {
	return &BetterStackProvider{
		name:       name,
		pageID:     pageID,
		apiToken:   apiToken,
		interval:   interval,
		timeout:    timeout,
		httpClient: NewHTTPClient(timeout),
	}
}

func (p *BetterStackProvider) Interval() time.Duration { return p.interval }
func (p *BetterStackProvider) Timeout() time.Duration  { return p.timeout }

// Minimal structs
type bsResources struct {
	Data []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Attr struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"attributes"`
	} `json:"data"`
}

func (p *BetterStackProvider) Fetch(ctx context.Context) (Result, error) {
	out := Result{Provider: "betterstack", Page: p.name}
	if p.apiToken == "" || p.pageID == "" {
		return out, fmt.Errorf("betterstack requires api_token and page_id")
	}
	// List status page resources
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://uptime.betterstack.com/api/v2/status-pages/%s/resources", p.pageID), nil)
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	res, err := p.httpClient.Do(req)
	if err != nil {
		return out, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return out, fmt.Errorf("unexpected status: %s", res.Status)
	}
	var r bsResources
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return out, err
	}
	for _, d := range r.Data {
		out.Components = append(out.Components, Component{
			Name:   d.Attr.Name,
			Status: mapBetterStack(d.Attr.Status),
		})
	}
	return out, nil
}

func mapBetterStack(s string) NormalizedStatus {
	switch strings.ToLower(s) {
	case "operational":
		return StatusOperational
	case "maintenance":
		return StatusUnderMaintenance
	case "degraded_performance", "degraded":
		return StatusDegraded
	case "partial_outage":
		return StatusPartialOutage
	case "major_outage", "outage", "down":
		return StatusMajorOutage
	default:
		return StatusUnknown
	}
}
