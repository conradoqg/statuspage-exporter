package providers

import (
    "context"
    "encoding/xml"
    "fmt"
    "net/http"
    "strings"
    "time"

    "github.com/conradoqg/statuspage-exporter/internal/logx"
)

// Status.io provider via RSS feed
type StatusIOProvider struct {
    name     string
    rssURL   string
    interval time.Duration
    timeout  time.Duration
    client   *http.Client
}

func NewStatusIO(name, rssURL string, interval, timeout time.Duration) *StatusIOProvider {
    return &StatusIOProvider{
        name:     name,
        rssURL:   rssURL,
        interval: interval,
        timeout:  timeout,
        client:   NewHTTPClient(timeout),
    }
}

func (p *StatusIOProvider) Interval() time.Duration { return p.interval }
func (p *StatusIOProvider) Timeout() time.Duration  { return p.timeout }

type statusioRSS struct {
    Channel struct {
        Item []struct {
            Title       string `xml:"title"`
            Description string `xml:"description"`
        } `xml:"item"`
    } `xml:"channel"`
}

func (p *StatusIOProvider) Fetch(ctx context.Context) (Result, error) {
    logx.Debugf("statusio(rss) fetch url=%s", p.rssURL)
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.rssURL, nil)
    res, err := p.client.Do(req)
    if err != nil {
        return Result{Provider: "statusio", Page: p.name}, err
    }
    defer res.Body.Close()
    if res.StatusCode != http.StatusOK {
        return Result{Provider: "statusio", Page: p.name}, fmt.Errorf("unexpected status: %s", res.Status)
    }
    var feed statusioRSS
    if err := xml.NewDecoder(res.Body).Decode(&feed); err != nil {
        return Result{Provider: "statusio_rss", Page: p.name}, err
    }
    out := Result{Provider: "statusio_rss", Page: p.name}
    // Map latest item to a page-level status using heuristics
    if len(feed.Channel.Item) > 0 {
        latest := feed.Channel.Item[0]
        st := inferStatusIOFromText(latest.Title + " " + latest.Description)
        out.Components = append(out.Components, Component{
            Name:   "",
            Status: st,
        })
    }
    logx.Debugf("statusio(rss) items=%d page=%s", len(feed.Channel.Item), p.name)
    return out, nil
}

func inferStatusIOFromText(s string) NormalizedStatus {
    t := strings.ToLower(s)
    // Order matters: check strongest signals first
    if strings.Contains(t, "major outage") || strings.Contains(t, "service disruption") || strings.Contains(t, "outage") || strings.Contains(t, "unavailable") || strings.Contains(t, "down") {
        return StatusMajorOutage
    }
    if strings.Contains(t, "maintenance") || strings.Contains(t, "under maintenance") {
        return StatusUnderMaintenance
    }
    if strings.Contains(t, "partial") || strings.Contains(t, "some users") || strings.Contains(t, "degraded") || strings.Contains(t, "increased errors") {
        return StatusPartialOutage
    }
    if strings.Contains(t, "operational") || strings.Contains(t, "resolved") || strings.Contains(t, "restored") || strings.Contains(t, "back to normal") {
        return StatusOperational
    }
    return StatusUnknown
}
