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

type StatuspageProvider struct {
	name     string
	baseURL  string
	interval time.Duration
	timeout  time.Duration
	client   *http.Client
}

func NewStatuspage(name, baseURL string, interval, timeout time.Duration) *StatuspageProvider {
	if !strings.HasPrefix(baseURL, "http") {
		baseURL = "https://" + baseURL
	}
	return &StatuspageProvider{
		name:     name,
		baseURL:  strings.TrimRight(baseURL, "/"),
		interval: interval,
		timeout:  timeout,
		client:   NewHTTPClient(timeout),
	}
}

func (p *StatuspageProvider) Interval() time.Duration { return p.interval }
func (p *StatuspageProvider) Timeout() time.Duration  { return p.timeout }

type spSummary struct {
	Components []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Status  string `json:"status"`
		Group   bool   `json:"group"`
		GroupID string `json:"group_id"`
	} `json:"components"`
	Incidents             []json.RawMessage `json:"incidents"`
	UnresolvedIncidents   []json.RawMessage `json:"unresolved_incidents"`
	ScheduledMaintenances []json.RawMessage `json:"scheduled_maintenances"`
}

func (p *StatuspageProvider) Fetch(ctx context.Context) (Result, error) {
	logx.Debugf("statuspage fetch base=%s", p.baseURL)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/api/v2/summary.json", nil)
	res, err := p.client.Do(req)
	if err != nil {
		return Result{Provider: "statuspage", Page: p.name}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Result{Provider: "statuspage", Page: p.name}, fmt.Errorf("unexpected status: %s", res.Status)
	}
	var s spSummary
	if err := json.NewDecoder(res.Body).Decode(&s); err != nil {
		return Result{Provider: "statuspage", Page: p.name}, err
	}
	out := Result{Provider: "statuspage", Page: p.name}
	// Map group id -> group name
	groups := make(map[string]string)
	for _, c := range s.Components {
		if c.Group {
			groups[c.ID] = c.Name
		}
	}
	// Add non-group components, carrying their parent group name if any
	for _, c := range s.Components {
		if c.Group {
			continue
		}
		out.Components = append(out.Components, Component{
			Name:   c.Name,
			Group:  groups[c.GroupID],
			Status: mapStatuspage(c.Status),
		})
	}
	// Determine open incidents: prefer explicit unresolved incidents from API if present.
	open := 0
	if len(s.UnresolvedIncidents) > 0 {
		open = len(s.UnresolvedIncidents)
	} else if len(s.Incidents) > 0 {
		// Many pages return only `incidents` in summary.json as unresolved
		open = len(s.Incidents)
	} else {
		// Fallback: count non-operational components (excluding maintenance)
		for _, comp := range out.Components {
			if comp.Status != StatusOperational && comp.Status != StatusUnderMaintenance {
				open++
			}
		}
	}
	out.OpenIncidents = open
	logx.Debugf("statuspage parsed components=%d open_incidents=%d page=%s", len(out.Components), open, p.name)
	return out, nil
}

func mapStatuspage(s string) NormalizedStatus {
	switch s {
	case "operational":
		return StatusOperational
	case "under_maintenance":
		return StatusUnderMaintenance
	case "degraded_performance":
		return StatusDegraded
	case "partial_outage":
		return StatusPartialOutage
	case "major_outage":
		return StatusMajorOutage
	default:
		return StatusUnknown
	}
}
