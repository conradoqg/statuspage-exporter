package providers

import (
    "context"
    "encoding/xml"
    "fmt"
    "net/http"
    "strings"
    "time"
)

// AWS public Service Health Dashboard does not provide an unauthenticated JSON summary.
// We support configuring specific RSS feeds and infer the latest status per feed.
// If the latest item suggests normal operation/resolved, we mark Operational; otherwise Degraded.
type AWSRSSProvider struct {
    name     string
    feeds    []FeedInput
    interval time.Duration
    timeout  time.Duration
    client   *http.Client
}

type FeedInput struct {
    URL     string
    Service string
    Region  string
}

func NewAWSRSS(name string, feeds []FeedInput, interval, timeout time.Duration) *AWSRSSProvider {
    return &AWSRSSProvider{
        name:     name,
        feeds:    feeds,
        interval: interval,
        timeout:  timeout,
        client:   NewHTTPClient(timeout),
    }
}

func (p *AWSRSSProvider) Interval() time.Duration { return p.interval }
func (p *AWSRSSProvider) Timeout() time.Duration  { return p.timeout }

type rss struct {
    Channel struct {
        Item []struct {
            Title       string `xml:"title"`
            Description string `xml:"description"`
            PubDate     string `xml:"pubDate"`
        } `xml:"item"`
    } `xml:"channel"`
}

func (p *AWSRSSProvider) Fetch(ctx context.Context) (Result, error) {
    out := Result{Provider: "aws_rss", Page: p.name}
    for _, f := range p.feeds {
        req, _ := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
        res, err := p.client.Do(req)
        if err != nil {
            return out, err
        }
        var r rss
        if err := xml.NewDecoder(res.Body).Decode(&r); err != nil {
            res.Body.Close()
            return out, err
        }
        res.Body.Close()
        st := inferAWSStatus(r)
        name := f.Service
        if f.Region != "" {
            name = fmt.Sprintf("%s (%s)", f.Service, f.Region)
        }
        out.Components = append(out.Components, Component{
            Name:   name,
            Region: f.Region,
            Status: st,
        })
    }
    return out, nil
}

func inferAWSStatus(r rss) NormalizedStatus {
    if len(r.Channel.Item) == 0 {
        return StatusUnknown
    }
    latest := r.Channel.Item[0]
    t := strings.ToLower(latest.Title + " " + latest.Description)
    if strings.Contains(t, "operating normally") || strings.Contains(t, "resolved") || strings.Contains(t, "restored") {
        return StatusOperational
    }
    if strings.Contains(t, "degraded") || strings.Contains(t, "elevated") || strings.Contains(t, "increased") || strings.Contains(t, "impact") {
        return StatusDegraded
    }
    if strings.Contains(t, "outage") || strings.Contains(t, "unavailable") {
        return StatusMajorOutage
    }
    return StatusUnknown
}

