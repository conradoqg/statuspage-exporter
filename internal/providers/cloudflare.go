package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/conradoqg/statuspage-exporter/internal/logx"
)

type CloudflareProvider struct {
	name         string
	baseURL      string
	endpointMode bool // true when config provided a full endpoint (path present)
	interval     time.Duration
	timeout      time.Duration
	client       *http.Client
}

func NewCloudflare(name, rawURL string, interval, timeout time.Duration) *CloudflareProvider {
	if rawURL == "" {
		rawURL = "https://www.cloudflarestatus.com"
	}
	if !strings.HasPrefix(rawURL, "http") {
		rawURL = "https://" + rawURL
	}
	endpointMode := false
	if u, err := url.Parse(rawURL); err == nil {
		if u.Path != "" && u.Path != "/" {
			endpointMode = true
		}
	}
	return &CloudflareProvider{
		name:         name,
		baseURL:      strings.TrimRight(rawURL, "/"),
		endpointMode: endpointMode,
		interval:     interval,
		timeout:      timeout,
		client:       NewHTTPClient(timeout),
	}
}

func (p *CloudflareProvider) Interval() time.Duration { return p.interval }
func (p *CloudflareProvider) Timeout() time.Duration  { return p.timeout }

// Minimal summary shape (same as Statuspage summary.json)
type cfSummary struct {
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

// incidents shape (when a caller points to incidents.json)
type cfIncidentComponent struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type cfIncident struct {
	ID         string                `json:"id"`
	Name       string                `json:"name"`
	Status     string                `json:"status"`
	Impact     string                `json:"impact"`
	Components []cfIncidentComponent `json:"components"`
}

type cfIncidents struct {
	Incidents []cfIncident `json:"incidents"`
}

func (p *CloudflareProvider) Fetch(ctx context.Context) (Result, error) {
	logx.Debugf("cloudflare fetch base=%s", p.baseURL)
	fetchURL := p.baseURL
	if !p.endpointMode {
		fetchURL = p.baseURL + "/api/v2/summary.json"
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	req.Header.Set("Accept", "application/json")
	res, err := p.client.Do(req)
	if err != nil {
		return Result{Provider: "cloudflare", Page: p.name}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Result{Provider: "cloudflare", Page: p.name}, fmt.Errorf("unexpected status: %s", res.Status)
	}
	// Read body to validate content and provide better error on HTML responses
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return Result{Provider: "cloudflare", Page: p.name}, err
	}
	// Quick check: many failures return HTML (body starts with '<') or non-JSON content
	if !json.Valid(body) {
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return Result{Provider: "cloudflare", Page: p.name}, fmt.Errorf("invalid JSON response (maybe HTML). snippet=%q", snippet)
	}
	var s cfSummary
	if err := json.Unmarshal(body, &s); err != nil {
		return Result{Provider: "cloudflare", Page: p.name}, err
	}

	out := Result{Provider: "cloudflare", Page: p.name}

	// If the response contains components (summary.json style), handle like Statuspage
	if len(s.Components) > 0 {
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
			open = len(s.Incidents)
		} else {
			for _, comp := range out.Components {
				if comp.Status != StatusOperational && comp.Status != StatusUnderMaintenance {
					open++
				}
			}
		}
		out.OpenIncidents = open
		logx.Debugf("cloudflare(parsed summary) components=%d open_incidents=%d page=%s", len(out.Components), open, p.name)
		return out, nil
	}

	// If no components present but incidents array exists (incidents.json), parse incidents
	if len(s.Incidents) > 0 {
		var incs cfIncidents
		if err := json.Unmarshal(body, &incs); err != nil {
			return Result{Provider: "cloudflare", Page: p.name}, err
		}
		open := 0
		for _, ic := range incs.Incidents {
			// treat any listed incident as open unless status indicates resolved
			stLower := strings.ToLower(strings.TrimSpace(ic.Status))
			if strings.Contains(stLower, "resolved") || strings.Contains(stLower, "closed") {
				continue
			}
			open++
			// If incident includes affected components, add them
			if len(ic.Components) > 0 {
				for _, ac := range ic.Components {
					out.Components = append(out.Components, Component{
						Name:   ac.Name,
						Status: mapCloudflareIncident(ac.Status),
					})
				}
				continue
			}
			// Otherwise, add a synthetic component per incident with status derived from impact/status
			out.Components = append(out.Components, Component{
				Name:   ic.Name,
				Status: mapCloudflareIncident(ic.Status + " " + ic.Impact),
			})
		}
		out.OpenIncidents = open
		logx.Debugf("cloudflare(parsed incidents) components=%d open_incidents=%d page=%s", len(out.Components), open, p.name)
		return out, nil
	}
	logx.Debugf("cloudflare parsed components=%d page=%s", len(out.Components), p.name)
	return out, nil
}

func mapCloudflareIncident(s string) NormalizedStatus {
	t := strings.ToLower(strings.TrimSpace(s))
	if t == "" {
		return StatusUnknown
	}
	if strings.Contains(t, "resolved") || strings.Contains(t, "fixed") || strings.Contains(t, "completed") {
		return StatusOperational
	}
	if strings.Contains(t, "maintenance") || strings.Contains(t, "scheduled") {
		return StatusUnderMaintenance
	}
	if strings.Contains(t, "major") || strings.Contains(t, "outage") || strings.Contains(t, "down") || strings.Contains(t, "critical") || strings.Contains(t, "unavailable") {
		return StatusMajorOutage
	}
	if strings.Contains(t, "partial") || strings.Contains(t, "degraded") || strings.Contains(t, "degradation") || strings.Contains(t, "minor") {
		return StatusPartialOutage
	}
	// Fallback: if wording suggests impact, mark degraded, otherwise unknown
	if strings.Contains(t, "impact") || strings.Contains(t, "issues") || strings.Contains(t, "service") {
		return StatusDegraded
	}
	return StatusUnknown
}
