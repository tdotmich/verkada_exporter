# How it works

## Data flow

Verkada is **pull**: we ask the API for the data. Prometheus is also pull. So the
exporter calls Verkada each time Prometheus scrapes `/metrics`:

```
 Prometheus ---- GET /metrics ----> Exporter
                                       |
                                       | 1. POST /token              (only when the token is old)
                                       |    x-api-key: <API key>
                                       | 2. GET /cameras/v1/devices  (each page, 200 cameras)
                                       |    x-verkada-auth: <token>
                                       v
                                    Verkada API
```

Compare this with the Talkdesk exporter. Talkdesk is **push**: it sends values on an
open stream, so that exporter stores the newest values in gauges. Verkada has no stream,
so this exporter gets fresh data on each scrape and stores nothing.

## Code layout

| File | What it does |
|---|---|
| `cmd/verkada_exporter/main.go` | Start-up. Reads the flags and the API key, registers the collector, starts the `/metrics` server. |
| `internal/verkada/client.go` | The HTTP client. Gets and reuses the API token, and sends authenticated GET requests. |
| `internal/verkada/cameras.go` | The `Camera` type and `ListCameras`, which reads every page of cameras. |
| `internal/collector/collector.go` | The Prometheus collector. On each scrape it lists the cameras and makes the metrics. |

## Design decisions

- **Custom collector, not gauges.** The data is fetched at scrape time, so the collector
  makes new metrics each time (`MustNewConstMetric`). With gauges, a removed camera would
  stay on `/metrics` with its old value.
- **`verkada_up` instead of a failed scrape.** If Verkada fails, the scrape still works and
  `verkada_up` is 0. This tells you the difference between "the exporter is down"
  (`up == 0`) and "Verkada is not answering" (`verkada_up == 0`).
- **Token reuse.** A token is valid for 30 minutes. We keep it for 25 minutes, so a token
  never expires during a scrape. If Verkada still says 401, we get a new token and try once more.
- **The API key goes only to `/token`.** Verkada also accepts the key on other endpoints,
  but we do not do that. The key is the long-lived secret, so it is sent as little as possible.
- **No redirects.** Go keeps custom headers such as `x-api-key` when it follows a redirect,
  also to another host. The client refuses redirects, so the key cannot leak this way.
- **A mutex on the token.** Two Prometheus servers can scrape at the same time. The mutex
  stops two goroutines from changing the token at once, and stops two token requests at once.
- **`CameraLister` interface.** The collector only needs `ListCameras`. The tests give it
  a fake, so they do not need a Verkada account.
- **Name and site on every metric.** The Prometheus way is to put these only on an info
  metric and join. That is hard for new users, so every camera metric has `camera_id`,
  `name` and `site`. Model, serial and firmware are only on `verkada_camera_info`.
- **No IP addresses or locations.** The API returns the local IP and the map position of
  each camera. These show the network and the building layout, so we do not export them.
- **Rate limit.** Verkada allows 300 requests per minute for each organization. A scrape
  uses one request for each 200 cameras, so a 60 second scrape interval is safe.
- **Exporter toolkit for the web server.** It is the standard Prometheus web server. It gives
  TLS and basic auth through `-web.config.file`, the same as `node_exporter`.

## Port

The default port is 10061. Ports 9100 to 9999 in the
[Prometheus port list](https://github.com/prometheus/prometheus/wiki/Default-port-allocations)
are all taken. 10060 is used by the Talkdesk exporter, and 10061 is the next free number.
