# Verkada Exporter

[![CI](https://github.com/tdotmich/verkada_exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/tdotmich/verkada_exporter/actions/workflows/ci.yml)

A Prometheus exporter for [Verkada](https://www.verkada.com). It shows on a `/metrics`
endpoint, so Prometheus, Grafana or an OpenTelemetry Collector can use it:

- **Cameras:** online or offline, last online time, retention, firmware.
- **Environmental sensors** (SV11, SV23, SV25): temperature, humidity, CO₂, PM2.5,
  vape index, noise and the other readings Verkada gives.
- **Viewing stations:** online or offline, last seen time.

Verkada does not supply a Prometheus exporter. This project fills that gap.

> This is a community project. It is not made, supported or endorsed by Verkada.

## How it works

Each time Prometheus scrapes `/metrics`, the exporter calls the
[Verkada API](https://apidocs.verkada.com) and returns fresh metrics. A device removed
from Verkada disappears at the next scrape.

The exporter swaps your API key for a short-lived API token and reuses the token for
25 minutes. The API key goes only to the token endpoint.

More detail: [docs/how-it-works.md](docs/how-it-works.md).

## What you need

1. A Verkada organization, and a Command user who can create API keys.
2. An API key with **read-only** access to **Cameras**, and to sensors and viewing
   stations if you export them. Give the key no other access.
   See the [Verkada API docs](https://apidocs.verkada.com).
3. For environmental sensors: the ID of each sensor. Verkada has no API that lists
   sensors, so you give the IDs to the exporter. The ID is shown on the sensor's page
   in Verkada Command.
4. Your region's API URL:

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

With viewing stations and two sensors:

```bash
verkada_exporter -verkada.viewing-stations -verkada.sensor-ids "SENSOR_ID_1,SENSOR_ID_2"
```

| Flag | Default | Description |
|---|---|---|
| `-verkada.api-url` | `https://api.verkada.com` | Verkada API URL for your region. |
| `-verkada.timeout` | `20s` | Time limit for the Verkada API calls in one scrape. |
| `-verkada.viewing-stations` | off | Also export viewing station status. |
| `-verkada.sensor-ids` | (none) | Comma-separated environmental sensor IDs to export. |
| `-web.listen-address` | `:10061` | Address for `/metrics`. |
| `-web.config.file` | (none) | Turns on TLS and basic auth. See [examples/web-config.yml](examples/web-config.yml). |
| `-version` | | Prints the version. |

## Metrics

| Metric | Type | Description |
|---|---|---|
| `verkada_up` | gauge | 1 when all the Verkada API calls in the last scrape worked, 0 when one or more failed. |
| `verkada_scrape_duration_seconds` | gauge | Time to get the data from Verkada. |
| `verkada_camera_online` | gauge | 1 when the camera status is Live, 0 when not. |
| `verkada_camera_last_online_timestamp_seconds` | gauge | Unix time when the camera was last online. |
| `verkada_camera_device_retention_days` | gauge | Days of video kept on the camera. |
| `verkada_camera_cloud_retention_days` | gauge | Days of video kept in the cloud. Not shown for a camera with no cloud backup. |
| `verkada_camera_people_history_enabled` | gauge | 1 when people history is on. |
| `verkada_camera_vehicle_history_enabled` | gauge | 1 when vehicle history is on. |
| `verkada_camera_info` | gauge | Always 1. Extra labels: `model`, `serial`, `firmware`. |

Every camera metric has the labels `camera_id`, `name` and `site`.

**Viewing stations** (with `-verkada.viewing-stations`):

| Metric | Type | Description |
|---|---|---|
| `verkada_viewing_station_online` | gauge | 1 when the status is online, 0 when not. |
| `verkada_viewing_station_last_seen_timestamp_seconds` | gauge | Unix time when it was last online. |
| `verkada_viewing_station_info` | gauge | Always 1. Extra labels: `serial`, `app_version`. |

Labels: `device_id`, `name`, `site_id`. (The viewing station API gives a site ID, not a site name.)

**Environmental sensors** (with `-verkada.sensor-ids`):

| Metric | Type | Description |
|---|---|---|
| `verkada_sensor_reading` | gauge | Newest reading. The `reading` label says what it measures. |
| `verkada_sensor_reporting` | gauge | 1 when the sensor sent a reading in the last 10 minutes, 0 when not. |
| `verkada_sensor_last_reading_timestamp_seconds` | gauge | Unix time of the newest reading. |

Labels: `sensor_id`, `name`, `site`. The `reading` label is one of: `temperature`,
`humidity`, `heat_index`, `carbon_dioxide`, `carbon_monoxide`, `formaldehyde`,
`pm_1_0_0`, `pm_2_5`, `pm_4_0`, `tvoc`, `tvoc_index`, `usa_air_quality_index`,
`vape_index`, `noise_level`, `barometric_pressure`, `ambient_light`, `motion`, `tamper`.
A sensor only has the readings its model supports.

Values are in the units Verkada uses. The Verkada API docs do not state the units,
so check them against Verkada Command before you set alert levels.

## Use with Prometheus

Add a scrape job. Each scrape calls the Verkada API, and Verkada allows 300 requests
per minute for each organization. One scrape uses one request for each 200 cameras,
one for the viewing stations, and one for each sensor. Scrape once a minute or less often:

```yaml
scrape_configs:
  - job_name: verkada
    scrape_interval: 60s
    scrape_timeout: 30s
    static_configs:
      - targets: ["exporter-host:10061"]
```

Example alerts (camera offline, sensor not reporting, high temperature, API failing) are in
[examples/alerts.yml](examples/alerts.yml). To try the exporter and Prometheus together,
use [examples/docker-compose.yml](examples/docker-compose.yml).

Useful queries:

```promql
# Offline cameras for each site
count by (site) (verkada_camera_online == 0)

# Cameras that have been offline for more than 1 hour
time() - verkada_camera_last_online_timestamp_seconds > 3600

# Temperature for each sensor
verkada_sensor_reading{reading="temperature"}
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
- Device IP addresses and map locations are not exported.
- `/metrics` contains device names, sites, serial numbers and sensor readings. Use `-web.config.file`
  for TLS and basic auth, or keep the port on a private network.
- The Docker image is distroless and runs as a non-root user.

To report a vulnerability, see [SECURITY.md](SECURITY.md).

## Limits

- Doors (Access Control) and Classic alarm devices are not exported. Their API
  endpoints return only names and types, with no status, so there is nothing to monitor.
- Environmental sensors must be listed by ID, because Verkada has no API to list them.
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
