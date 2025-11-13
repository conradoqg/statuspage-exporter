# statuspage-exporter

Prometheus exporter for vendor status pages. Scrapes multiple status page providers, normalizes component statuses, and exposes metrics.

Supported providers:

- Atlassian Statuspage (generic) — e.g., Twilio, Datadog, CloudAMQP, MongoDB Atlas
- Instatus (generic) — e.g., Fluig Identity and any `*.instatus.com`
- Status.io (generic Public Status API — per-page endpoint required)
- Azure DevOps — public Health API
- Google Cloud — incidents JSON (inferred current impacts)
- AWS — RSS feeds (per-service/region)
- Better Stack — REST API with token + status page ID

## Build and run

```
go build ./cmd/statuspage-exporter
./statuspage-exporter --config=config.yaml --listen=":9090"
```

Metrics served at `/metrics`.

## Configuration

See `config.example.yaml` for a full example. Key fields:

- `server.listen`: HTTP listen address
- `common.interval`: default scrape interval
- `common.timeout`: default HTTP timeout
- `pages`: list of targets
  - `type`: one of `statuspage|instatus|statusio_rss|azuredevops|gcp|aws_rss|betterstack`
  - `url`: base URL or provider-specific endpoint
  - `user_friendly_url`: public status page URL to display in dashboards
  - `api_token` / `page_id`: used by Better Stack
  - `feeds`: used by `aws_rss` (list of RSS URLs with service/region labels)

### Provider notes

- Statuspage: Uses `GET <base>/api/v2/summary.json`. Components are exported as-is (groups are skipped).
- Instatus: Prefers `GET <base>/v2/components.json`, falls back to `GET <base>/summary.json`.
- Status.io (RSS): Set `type: statusio_rss` and the page RSS feed (e.g., `https://status.status.io/pages/<PAGE_ID>/rss`). We infer a page-level status from the latest item’s title/description.
- Azure DevOps: Uses `GET https://status.dev.azure.com/_apis/status/health?api-version=7.1-preview.1`.
- Google Cloud: Uses `GET https://status.cloud.google.com/incidents.json`. Only active incidents emit components; otherwise no components are emitted (no canonical per-product summary endpoint).
- AWS: Public dashboard offers RSS feeds. Configure the feeds you care about. Latest item content is heuristically mapped to a status.
- Better Stack: Requires token and status page ID. Resources are fetched from `GET /api/v2/status-pages/{page_id}/resources`.

## Metrics

- `statuspage_component_up{provider,page,component,group,region}` — 1 if operational, else 0
- `statuspage_component_status_code{provider,page,component,group,region,status}` — normalized code
  - 0=unknown, 1=operational, 2=maintenance, 3=degraded, 4=partial_outage, 5=major_outage
- `statuspage_open_incidents{provider,page}` — open incidents when available
- `statuspage_scrape_duration_seconds{provider,page}` — scrape duration
- `statuspage_scrape_success{provider,page}` — 1 if scrape succeeded
- `statuspage_page_info{provider,page,url}` — static info metric (value 1) you can use to display a link to the vendor’s official status page

## Mapping references (public docs)

- Statuspage `summary.json`: `/api/v2/summary.json`
- Instatus Components: `/v2/components.json` (legacy `/summary.json`)
- Status.io Public Status API: per-page endpoint; returns `status_code` values (100 ok, 200 maintenance, 300 degraded, 400 partial, 500 disruption, 600 security).
- Azure DevOps Health API: `_apis/status/health?api-version=7.1-preview.1`
- Google Cloud incidents JSON: `https://status.cloud.google.com/incidents.json`
- AWS Service Health RSS: per-service/region feeds under `https://status.aws.amazon.com/rss/`
- Better Stack Uptime API: `https://uptime.betterstack.com/api/v2/` (requires token)

## Example targets

Common pages you can try:

- MongoDB Atlas: https://status.cloud.mongodb.com (Statuspage)
- Twilio: https://status.twilio.com (Statuspage)
- Datadog: https://status.datadoghq.com (Statuspage)
- CloudAMQP: https://status.cloudamqp.com (Statuspage)
- Fluig Identity: https://fluig-identity.instatus.com (Instatus)
- Azure DevOps: https://status.dev.azure.com/_apis/status/health?api-version=7.1-preview.1
- Google Cloud: https://status.cloud.google.com/incidents.json
- AWS RSS examples: https://status.aws.amazon.com/rss/cloudfront.rss, https://status.aws.amazon.com/rss/ec2-ap-southeast-1.rss

## Caveats

- Some vendors do not publish a canonical unauthenticated status summary (e.g., AWS). For those, RSS or authenticated APIs are used.
- Google Cloud incidents JSON reflects active and historical incidents, not a permanent per-product status ledger. We only emit impacted components when incidents are open.
- Better Stack requires an API token for programmatic access.
## Containers

- Build: `make docker-build`
- Run: `make docker-run`

## Kubernetes

- Apply manifests (namespace `statuspage`): `kubectl apply -f k8s/statuspage-exporter.yaml -n statuspage`
- ServiceMonitor assumes Prometheus Operator label `release=prometheus`.

## Grafana Dashboard

- Import dashboards/statuspage-exporter-dashboard.json into Grafana.
