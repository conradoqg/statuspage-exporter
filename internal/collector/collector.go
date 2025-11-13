package collector

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/prometheus/client_golang/prometheus"

    "github.com/conradoqg/statuspage-exporter/internal/config"
    "github.com/conradoqg/statuspage-exporter/internal/providers"
)

type Exporter struct {
    providers []providers.Provider
    caches    []*cacheEntry
    metas     []pageMeta

    up         *prometheus.Desc
    statusCode *prometheus.Desc
    scrapeDur  *prometheus.Desc
    scrapeOK   *prometheus.Desc
    incidents  *prometheus.Desc
    pageInfo   *prometheus.Desc
}

type cacheEntry struct {
    mu      sync.RWMutex
    res     providers.Result
    err     error
    dur     float64
    updated time.Time
}

type pageMeta struct {
    Provider string
    Page     string
    URL      string
}

func New(cfg *config.Config) (*Exporter, error) {
    ps, metas, err := buildProviders(cfg)
    if err != nil {
        return nil, err
    }
    e := &Exporter{
        providers: ps,
        caches:    make([]*cacheEntry, len(ps)),
        metas:     metas,
        up: prometheus.NewDesc(
            "statuspage_component_up",
            "Component operational status (1=up, 0=not)",
            []string{"provider", "page", "component", "group", "region"}, nil,
        ),
        statusCode: prometheus.NewDesc(
            "statuspage_component_status_code",
            "Component normalized status code (0=unknown,1=operational,2=maintenance,3=degraded,4=partial_outage,5=major_outage)",
            []string{"provider", "page", "component", "group", "region", "status"}, nil,
        ),
        scrapeDur: prometheus.NewDesc(
            "statuspage_scrape_duration_seconds",
            "Scrape duration by provider/page",
            []string{"provider", "page"}, nil,
        ),
        scrapeOK: prometheus.NewDesc(
            "statuspage_scrape_success",
            "Scrape success (1=ok)",
            []string{"provider", "page"}, nil,
        ),
        incidents: prometheus.NewDesc(
            "statuspage_open_incidents",
            "Open incidents reported by provider/page (when available)",
            []string{"provider", "page"}, nil,
        ),
        pageInfo: prometheus.NewDesc(
            "statuspage_page_info",
            "Static page info metric for dashboards; value is 1",
            []string{"provider", "page", "url"}, nil,
        ),
    }

    // Start background refresh loops respecting provider intervals
    for i, p := range e.providers {
        ce := &cacheEntry{}
        e.caches[i] = ce
        go e.refreshLoop(p, ce)
    }
    return e, nil
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
    ch <- e.up
    ch <- e.statusCode
    ch <- e.scrapeDur
    ch <- e.scrapeOK
    ch <- e.incidents
    ch <- e.pageInfo
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
    // Emit static page info for each configured target
    for _, m := range e.metas {
        ch <- prometheus.MustNewConstMetric(e.pageInfo, prometheus.GaugeValue, 1, m.Provider, m.Page, m.URL)
    }
    for i := range e.providers {
        e.collectFromCache(i, ch)
    }
}

func (e *Exporter) collectFromCache(i int, ch chan<- prometheus.Metric) {
    ce := e.caches[i]
    ce.mu.RLock()
    res := ce.res
    err := ce.err
    dur := ce.dur
    ce.mu.RUnlock()

    // If cache is empty (first run), do a synchronous fetch to avoid empty metrics
    if res.Provider == "" && err == nil {
        p := e.providers[i]
        start := time.Now()
        ctx, cancel := context.WithTimeout(context.Background(), p.Timeout())
        r, er := p.Fetch(ctx)
        cancel()
        dur = time.Since(start).Seconds()
        ce.mu.Lock()
        ce.res, ce.err, ce.dur, ce.updated = r, er, dur, time.Now()
        res, err = r, er
        ce.mu.Unlock()
    }

    if res.Provider == "" {
        // nothing to expose yet
        return
    }

    ch <- prometheus.MustNewConstMetric(e.scrapeDur, prometheus.GaugeValue, dur, res.Provider, res.Page)
    if err != nil {
        ch <- prometheus.MustNewConstMetric(e.scrapeOK, prometheus.GaugeValue, 0, res.Provider, res.Page)
        return
    }
    ch <- prometheus.MustNewConstMetric(e.scrapeOK, prometheus.GaugeValue, 1, res.Provider, res.Page)
    if res.OpenIncidents >= 0 {
        ch <- prometheus.MustNewConstMetric(e.incidents, prometheus.GaugeValue, float64(res.OpenIncidents), res.Provider, res.Page)
    }
    // Deduplicate by labelset to avoid duplicate series if a provider returns repeated items
    seenUp := make(map[string]struct{})
    seenStatus := make(map[string]struct{})
    for _, c := range res.Components {
        keyUp := fmt.Sprintf("%s|%s|%s|%s|%s", res.Provider, res.Page, c.Name, c.Group, c.Region)
        if _, ok := seenUp[keyUp]; !ok {
            up := 0.0
            if c.Status == providers.StatusOperational {
                up = 1.0
            }
            ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, up, res.Provider, res.Page, c.Name, c.Group, c.Region)
            seenUp[keyUp] = struct{}{}
        }
        keyStatus := keyUp + "|" + c.Status.String()
        if _, ok := seenStatus[keyStatus]; !ok {
            ch <- prometheus.MustNewConstMetric(e.statusCode, prometheus.GaugeValue, float64(mapCode(c.Status)), res.Provider, res.Page, c.Name, c.Group, c.Region, c.Status.String())
            seenStatus[keyStatus] = struct{}{}
        }
    }
}

func (e *Exporter) refreshLoop(p providers.Provider, ce *cacheEntry) {
    // initial immediate fetch
    for {
        start := time.Now()
        ctx, cancel := context.WithTimeout(context.Background(), p.Timeout())
        res, err := p.Fetch(ctx)
        cancel()
        ce.mu.Lock()
        ce.res = res
        ce.err = err
        ce.dur = time.Since(start).Seconds()
        ce.updated = time.Now()
        ce.mu.Unlock()
        time.Sleep(p.Interval())
    }
}

func mapCode(s providers.NormalizedStatus) int {
    switch s {
    case providers.StatusUnknown:
        return 0
    case providers.StatusOperational:
        return 1
    case providers.StatusUnderMaintenance:
        return 2
    case providers.StatusDegraded:
        return 3
    case providers.StatusPartialOutage:
        return 4
    case providers.StatusMajorOutage:
        return 5
    default:
        return 0
    }
}

func buildProviders(cfg *config.Config) ([]providers.Provider, []pageMeta, error) {
    var ps []providers.Provider
    var metas []pageMeta
    for _, p := range cfg.Pages {
        interval := cfg.Common.Interval
        timeout := cfg.Common.Timeout
        if p.Interval != nil {
            interval = *p.Interval
        }
        if p.Timeout != nil {
            timeout = *p.Timeout
        }
        friendly := p.UserFriendlyURL
        if friendly == "" {
            friendly = p.URL
        }
        switch p.Type {
        case "statuspage":
            ps = append(ps, providers.NewStatuspage(p.Name, p.URL, interval, timeout))
            metas = append(metas, pageMeta{Provider: "statuspage", Page: p.Name, URL: friendly})
        case "instatus":
            ps = append(ps, providers.NewInstatus(p.Name, p.URL, interval, timeout))
            metas = append(metas, pageMeta{Provider: "instatus", Page: p.Name, URL: friendly})
        case "statusio":
            ps = append(ps, providers.NewStatusIO(p.Name, p.URL, interval, timeout))
            metas = append(metas, pageMeta{Provider: "statusio", Page: p.Name, URL: friendly})
        case "azuredevops":
            ps = append(ps, providers.NewAzureDevOps(p.Name, p.URL, interval, timeout))
            metas = append(metas, pageMeta{Provider: "azuredevops", Page: p.Name, URL: friendly})
        case "gcp":
            ps = append(ps, providers.NewGCP(p.Name, p.URL, interval, timeout))
            metas = append(metas, pageMeta{Provider: "gcp", Page: p.Name, URL: friendly})
        case "aws_rss":
            feeds := make([]providers.FeedInput, 0, len(p.Feeds))
            for _, f := range p.Feeds {
                feeds = append(feeds, providers.FeedInput{URL: f.URL, Service: f.Service, Region: f.Region})
            }
            ps = append(ps, providers.NewAWSRSS(p.Name, feeds, interval, timeout))
            metas = append(metas, pageMeta{Provider: "aws_rss", Page: p.Name, URL: friendly})
        case "betterstack":
            ps = append(ps, providers.NewBetterStack(p.Name, p.PageID, p.APIToken, interval, timeout))
            metas = append(metas, pageMeta{Provider: "betterstack", Page: p.Name, URL: friendly})
        default:
            return nil, nil, fmt.Errorf("unknown provider type: %s", p.Type)
        }
    }
    return ps, metas, nil
}
