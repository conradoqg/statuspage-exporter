package providers

import (
    "context"
    "net/http"
    "time"
)

// NormalizedStatus represents a cross-vendor status bucket.
type NormalizedStatus int

const (
    StatusUnknown NormalizedStatus = iota
    StatusOperational
    StatusUnderMaintenance
    StatusDegraded
    StatusPartialOutage
    StatusMajorOutage
)

func (s NormalizedStatus) String() string {
    switch s {
    case StatusOperational:
        return "operational"
    case StatusUnderMaintenance:
        return "under_maintenance"
    case StatusDegraded:
        return "degraded_performance"
    case StatusPartialOutage:
        return "partial_outage"
    case StatusMajorOutage:
        return "major_outage"
    default:
        return "unknown"
    }
}

// Component describes a unit we expose as a metric.
type Component struct {
    Name   string
    Group  string
    Region string
    Status NormalizedStatus
}

type Result struct {
    Provider string
    Page     string
    // Components can be empty if not applicable
    Components []Component
    // OpenIncidents is optional
    OpenIncidents int
}

// Provider scrapes a single page/config and returns a normalized Result.
type Provider interface {
    Fetch(ctx context.Context) (Result, error)
    Interval() time.Duration
    Timeout() time.Duration
}

// HTTP wrapper with UA and timeout
func NewHTTPClient(timeout time.Duration) *http.Client {
    return &http.Client{Timeout: timeout}
}

