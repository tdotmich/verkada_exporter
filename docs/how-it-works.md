# How it works

## Data flow

Verkada is **pull**: we ask the API for the data. Prometheus is also pull. So the
exporter calls Verkada each time Prometheus scrapes `/metrics`:

```
 Prometheus ---- GET /metrics ----> Exporter
                                       |
                                       | 1. POST /token                     (only when the token is old)
                                       |    x-api-key: <API key>
                                       |
                                       |    All the calls below use x-verkada-auth: <token>
                                       | 2. GET /cameras/v1/devices         (each page, 200 cameras)
                                       | 3. GET /viewing_station/v1/devices (if turned on)
                                       | 4. GET /environment/v1/data        (one call for each sensor ID)
                                       v
                                    Verkada API
```

The exporter stores no device data between scrapes. Each scrape gets fresh data from Verkada.

## Code layout

| File | What it does |
|---|---|
| `cmd/verkada_exporter/main.go` | Start-up. Reads the flags and the API key, registers the collector, starts the `/metrics` server. |
| `internal/verkada/client.go` | The HTTP client. Gets and reuses the API token, and sends authenticated GET requests. |
| `internal/verkada/cameras.go` | The `Camera` type and `ListCameras`, which reads every page of cameras. |
| `internal/verkada/viewing_stations.go` | The `ViewingStation` type and `ListViewingStations`. |
| `internal/verkada/sensors.go` | `GetLatestSensorReading`, which gets the newest reading of one environmental sensor. |
| `internal/verkada/numbers.go` | `parseNumber`, which reads a value that Verkada sends as a number, a string or a boolean. |
| `internal/collector/collector.go` | The Prometheus collector. On each scrape it gets each kind of device and makes the metrics. |

## Design decisions

- **Custom collector, not gauges.** The data is fetched at scrape time, so the collector
  makes new metrics each time (`MustNewConstMetric`). With gauges, a removed device would
  stay on `/metrics` with its old value.
- **`verkada_up` instead of a failed scrape.** If Verkada fails, the scrape still works and
  `verkada_up` is 0. This tells you the difference between "the exporter is down"
  (`up == 0`) and "Verkada is not answering" (`verkada_up == 0`).
- **One failed part does not hide the others.** Cameras, viewing stations and each sensor
  are separate calls. If one fails, the log says which one, `verkada_up` is 0, and the
  metrics from the calls that worked still show.
- **Viewing stations are off by default.** The API key needs extra access for them. If they
  were on by default, a key with only camera access would make every scrape fail.
- **Sensor IDs come from a flag.** Verkada has no endpoint that lists environmental sensors,
  so the user gives the IDs. Repeated IDs are removed, because the same metric twice makes
  Prometheus reject the whole scrape.
- **One `verkada_sensor_reading` metric with a `reading` label.** The Verkada docs do not
  give the units of the readings, so a name such as `temperature_celsius` could be wrong.
  The `reading` label only takes names from a fixed list (`SensorReadingNames`), so new
  fields from the API cannot add new series.
- **The newest reading only.** We ask for the last 10 minutes at one row per minute and keep
  the newest row. A sensor with no rows has `verkada_sensor_reporting 0` and no values, so an
  old value is never shown as if it were new.
- **`parseNumber`.** The docs say some numbers are strings (for example `last_seen_at`), and
  some readings can be booleans (`motion`, `tamper`). One function reads all of these, so the
  exporter keeps working if Verkada changes the type.
- **Token reuse.** A token is valid for 30 minutes. We keep it for 25 minutes, so a token
  never expires during a scrape. If Verkada still says 401, we get a new token and try once more.
- **The API key goes only to `/token`.** Verkada also accepts the key on other endpoints,
  but we do not do that. The key is the long-lived secret, so it is sent as little as possible.
- **No redirects.** Go keeps custom headers such as `x-api-key` when it follows a redirect,
  also to another host. The client refuses redirects, so the key cannot leak this way.
- **A mutex on the token.** Two Prometheus servers can scrape at the same time. The mutex
  stops two goroutines from changing the token at once, and stops two token requests at once.
- **`VerkadaAPI` interface.** The collector only needs three methods from the client. The
  tests give it a fake, so they do not need a Verkada account.
- **Name and site on every metric.** The Prometheus way is to put these only on an info
  metric and join. That is hard for new users, so every device metric has its ID, name and
  site. Model, serial, firmware and app version are only on the `_info` metrics.
- **No IP addresses or locations.** The API returns the IP address and the map position of
  cameras and viewing stations. These show the network and the building layout, so we do
  not export them.
- **No doors or Classic alarm devices.** Their endpoints return only names and types, with
  no status. There is nothing to monitor.
- **Rate limit.** Verkada allows 300 requests per minute for each organization. A scrape uses
  one request for each 200 cameras, one for the viewing stations and one for each sensor,
  so a 60 second scrape interval is safe for a normal number of sensors.
- **Exporter toolkit for the web server.** It is the standard Prometheus web server. It gives
  TLS and basic auth through `-web.config.file`, the same as `node_exporter`.

## Port

The default port is 10061. Ports 9100 to 9999 in the
[Prometheus port list](https://github.com/prometheus/prometheus/wiki/Default-port-allocations)
are all taken, and no listed exporter uses 10061.
