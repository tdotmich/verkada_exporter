# Verkada Exporter

[![CI](https://github.com/tdotmich/verkada_exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/tdotmich/verkada_exporter/actions/workflows/ci.yml)

A Prometheus exporter for [Verkada](https://www.verkada.com) cameras. It shows the
health of every camera in your organization (online or offline, last online time,
retention, firmware) on a `/metrics` endpoint, so Prometheus, Grafana or an
OpenTelemetry Collector can use it.

Verkada does not supply a Prometheus exporter. This project fills that gap.

> This is a community project. It is not made, supported or endorsed by Verkada.

## How it works

Each time Prometheus scrapes `/metrics`, the exporter calls the
[Verkada API](https://apidocs.verkada.com) (`GET /cameras/v1/devices`, all pages)
and returns fresh metrics. A camera removed from Verkada disappears at the next scrape.

The exporter swaps your API key for a short-lived API token and reuses the token for
25 minutes. The API key goes only to the token endpoint.

More detail: [docs/how-it-works.md](docs/how-it-works.md).

## What you need

1. A Verkada organization, and a Command user who can create API keys.
2. An API key with **read-only** access to **Cameras**. Give the key no other access.
   See the [Verkada API docs](https://apidocs.verkada.com).
3. Your region's API URL:

   | Region | `-verkada.api-url` |
   |---|---|
   | US (default) | `https://api.verkada.com` |
   | EU | `https://api.eu.verkada.com` |
   | Australia | `https://api.au.verkada.com` |

## Install

**Binary.** Download a release for Linux, macOS or Windows from the
[releases page](https://github.com/tdotmich/verkada_exporter/releases).

**Docker.**

```bash
docker run -d -p 10061:10061 -e VERKADA_API_KEY ghcr.io/tdotmich/verkada_exporter:latest
```

**From source** (Go 1.27.3 or later).

```bash
go install github.com/tdotmich/verkada_exporter/cmd/verkada_exporter@latest
```

## Run

Set the API key as an environment variable. Do not put it on the command line:

```bash
export VERKADA_API_KEY=...
```

```bash
verkada_exporter -verkada.api-url https://api.verkada.com
```

| Flag | Default | Description |
|---|---|---|
| `-verkada.api-url` | `https://api.verkada.com` | Verkada API URL for your region. |
| `-verkada.timeout` | `20s` | Time limit for the Verkada API calls in one scrape. |
| `-web.listen-address` | `:10061` | Address for `/metrics`. |
| `-web.config.file` | (none) | Turns on TLS and basic auth. See [examples/web-config.yml](examples/web-config.yml). |
| `-version` | | Prints the version. |

## Metrics

| Metric | Type | Description |
|---|---|---|
| `verkada_up` | gauge | 1 when the last Verkada API call worked, 0 when it failed. |
| `verkada_scrape_duration_seconds` | gauge | Time to get the data from Verkada. |
| `verkada_camera_online` | gauge | 1 when the camera status is Live, 0 when not. |
| `verkada_camera_last_online_timestamp_seconds` | gauge | Unix time when the camera was last online. |
| `verkada_camera_device_retention_days` | gauge | Days of video kept on the camera. |
| `verkada_camera_cloud_retention_days` | gauge | Days of video kept in the cloud. Not shown for a camera with no cloud backup. |
| `verkada_camera_people_history_enabled` | gauge | 1 when people history is on. |
| `verkada_camera_vehicle_history_enabled` | gauge | 1 when vehicle history is on. |
| `verkada_camera_info` | gauge | Always 1. Extra labels: `model`, `serial`, `firmware`. |

Every camera metric has the labels `camera_id`, `name` and `site`.

## Use with Prometheus

Add a scrape job. Each scrape calls the Verkada API, and Verkada allows 300 requests
per minute for each organization. Scrape once a minute or less often:

```yaml
scrape_configs:
  - job_name: verkada
    scrape_interval: 60s
    scrape_timeout: 30s
    static_configs:
      - targets: ["exporter-host:10061"]
```

Example alerts (camera offline, API failing, firmware not up to date) are in
[examples/alerts.yml](examples/alerts.yml). To try the exporter and Prometheus together,
use [examples/docker-compose.yml](examples/docker-compose.yml).

Useful queries:

```promql
# Offline cameras for each site
count by (site) (verkada_camera_online == 0)

# Cameras that have been offline for more than 1 hour
time() - verkada_camera_last_online_timestamp_seconds > 3600
```

## Use with OpenTelemetry

Use the [Prometheus receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/prometheusreceiver)
in the OpenTelemetry Collector. It is in the `otelcol-contrib` distribution. The receiver
scrapes the exporter, and any metrics exporter (for example OTLP) sends the data on:

```yaml
receivers:
  prometheus:
    config:
      scrape_configs:
        - job_name: verkada
          scrape_interval: 60s
          scrape_timeout: 30s
          static_configs:
            - targets: ["exporter-host:10061"]

exporters:
  otlphttp:
    endpoint: https://otlp.example.com

service:
  pipelines:
    metrics:
      receivers: [prometheus]
      exporters: [otlphttp]
```

Full example: [examples/otel-collector.yaml](examples/otel-collector.yaml).

## Security

- The API key comes only from an environment variable. It goes only to `POST /token`,
  and the exporter never logs it or the token.
- The exporter does not follow redirects, so a redirect cannot send the key to another host.
- Camera IP addresses and map locations are not exported.
- `/metrics` contains camera names, sites and serial numbers. Use `-web.config.file`
  for TLS and basic auth, or keep the port on a private network.
- The Docker image is distroless and runs as a non-root user.

To report a vulnerability, see [SECURITY.md](SECURITY.md).

## Limits

- Only cameras are supported. Access control, alarms and environmental sensors are not.
- With two Prometheus servers (high availability), each one scrapes, so the API calls double.

## Development

```bash
go test ./...
```

The tests use a fake Verkada API. They do not need a Verkada account.

Issues and pull requests are welcome. This project is maintained on a best-effort basis.
If you need a change quickly, fork it. The license allows this.

## License

[MIT](LICENSE)
