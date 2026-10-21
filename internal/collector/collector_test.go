package collector

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/tdotmich/verkada_exporter/internal/verkada"
)

// fakeAPI returns fixed data, or errors, instead of calling Verkada.
type fakeAPI struct {
	cameras            []verkada.Camera
	camerasErr         error
	viewingStations    []verkada.ViewingStation
	viewingStationsErr error
	sensorReadings     map[string]*verkada.SensorReading
}

func (api fakeAPI) ListCameras(ctx context.Context) ([]verkada.Camera, error) {
	return api.cameras, api.camerasErr
}

func (api fakeAPI) ListViewingStations(ctx context.Context) ([]verkada.ViewingStation, error) {
	return api.viewingStations, api.viewingStationsErr
}

func (api fakeAPI) GetLatestSensorReading(ctx context.Context, sensorID string) (*verkada.SensorReading, error) {
	reading, exists := api.sensorReadings[sensorID]
	if !exists {
		return nil, errors.New("verkada API returned HTTP status 404 Not Found")
	}

	return reading, nil
}

func TestCollectCameraMetrics(t *testing.T) {
	cloudDays := 30.0

	api := fakeAPI{cameras: []verkada.Camera{
		{
			CameraID:             "cam-1",
			Name:                 "Front Door",
			Site:                 "HQ",
			Model:                "CD52",
			Serial:               "ABC123",
			Status:               "Live",
			Firmware:             "Up to date",
			LastOnline:           1700000000,
			DeviceRetentionDays:  30,
			CloudRetentionDays:   &cloudDays,
			PeopleHistoryEnabled: true,
		},
		{
			// An offline camera with no cloud backup and no last online time.
			CameraID:            "cam-2",
			Name:                "Car Park",
			Site:                "HQ",
			Status:              "Offline",
			DeviceRetentionDays: 7,
		},
	}}

	expected := `
# HELP verkada_up 1 when all the Verkada API calls in the last scrape worked, 0 when one or more failed.
# TYPE verkada_up gauge
verkada_up 1
# HELP verkada_camera_online 1 when the camera status is Live, 0 when it is not.
# TYPE verkada_camera_online gauge
verkada_camera_online{camera_id="cam-1",name="Front Door",site="HQ"} 1
verkada_camera_online{camera_id="cam-2",name="Car Park",site="HQ"} 0
# HELP verkada_camera_cloud_retention_days Days of video kept in the Verkada cloud.
# TYPE verkada_camera_cloud_retention_days gauge
verkada_camera_cloud_retention_days{camera_id="cam-1",name="Front Door",site="HQ"} 30
# HELP verkada_camera_last_online_timestamp_seconds Unix time when the camera was last online.
# TYPE verkada_camera_last_online_timestamp_seconds gauge
verkada_camera_last_online_timestamp_seconds{camera_id="cam-1",name="Front Door",site="HQ"} 1.7e+09
# HELP verkada_camera_info Information about the camera. The value is always 1.
# TYPE verkada_camera_info gauge
verkada_camera_info{camera_id="cam-1",firmware="Up to date",model="CD52",name="Front Door",serial="ABC123",site="HQ"} 1
verkada_camera_info{camera_id="cam-2",firmware="",model="",name="Car Park",serial="",site="HQ"} 1
`

	err := testutil.CollectAndCompare(
		New(api, Options{Timeout: time.Second}),
		strings.NewReader(expected),
		"verkada_up",
		"verkada_camera_online",
		"verkada_camera_cloud_retention_days",
		"verkada_camera_last_online_timestamp_seconds",
		"verkada_camera_info",
	)
	if err != nil {
		t.Error(err)
	}
}

func TestCollectAPIError(t *testing.T) {
	api := fakeAPI{camerasErr: errors.New("verkada API returned HTTP status 429 Too Many Requests")}

	expected := `
# HELP verkada_up 1 when all the Verkada API calls in the last scrape worked, 0 when one or more failed.
# TYPE verkada_up gauge
verkada_up 0
`

	err := testutil.CollectAndCompare(New(api, Options{Timeout: time.Second}), strings.NewReader(expected), "verkada_up")
	if err != nil {
		t.Error(err)
	}

	// No camera metrics on a failed scrape.
	count := testutil.CollectAndCount(New(api, Options{Timeout: time.Second}), "verkada_camera_online")
	if count != 0 {
		t.Errorf("expected no camera metrics, got %d", count)
	}
}

func TestCollectViewingStations(t *testing.T) {
	api := fakeAPI{viewingStations: []verkada.ViewingStation{
		{
			DeviceID:   "vs-1",
			Name:       "Security Desk",
			SiteID:     "site-1",
			Serial:     "VS123",
			LastStatus: "online",
			AppVersion: "2.4.0",
			LastSeenAt: json.RawMessage(`"1700000000"`),
		},
		{
			DeviceID:   "vs-2",
			Name:       "Lobby",
			SiteID:     "site-1",
			LastStatus: "offline",
		},
	}}

	expected := `
# HELP verkada_viewing_station_online 1 when the viewing station status is online, 0 when it is not.
# TYPE verkada_viewing_station_online gauge
verkada_viewing_station_online{device_id="vs-1",name="Security Desk",site_id="site-1"} 1
verkada_viewing_station_online{device_id="vs-2",name="Lobby",site_id="site-1"} 0
# HELP verkada_viewing_station_last_seen_timestamp_seconds Unix time when the viewing station was last online.
# TYPE verkada_viewing_station_last_seen_timestamp_seconds gauge
verkada_viewing_station_last_seen_timestamp_seconds{device_id="vs-1",name="Security Desk",site_id="site-1"} 1.7e+09
`

	err := testutil.CollectAndCompare(
		New(api, Options{Timeout: time.Second, ViewingStations: true}),
		strings.NewReader(expected),
		"verkada_viewing_station_online",
		"verkada_viewing_station_last_seen_timestamp_seconds",
	)
	if err != nil {
		t.Error(err)
	}
}

func TestViewingStationsOffByDefault(t *testing.T) {
	// If the API key has no viewing station access, this call fails.
	// With the option off, it must not be called, so verkada_up stays 1.
	api := fakeAPI{viewingStationsErr: errors.New("verkada API returned HTTP status 403 Forbidden")}

	expected := `
# HELP verkada_up 1 when all the Verkada API calls in the last scrape worked, 0 when one or more failed.
# TYPE verkada_up gauge
verkada_up 1
`

	err := testutil.CollectAndCompare(New(api, Options{Timeout: time.Second}), strings.NewReader(expected), "verkada_up")
	if err != nil {
		t.Error(err)
	}
}

func TestCollectSensors(t *testing.T) {
	api := fakeAPI{sensorReadings: map[string]*verkada.SensorReading{
		"sensor-1": {
			SensorID: "sensor-1",
			Name:     "Server Room",
			Site:     "HQ",
			Time:     1700000000,
			Values:   map[string]float64{"temperature": 21.5, "humidity": 40},
		},
		// A sensor that sent nothing in the last 10 minutes.
		"sensor-2": {
			SensorID: "sensor-2",
			Name:     "Bathroom",
			Site:     "HQ",
			Values:   map[string]float64{},
		},
	}}

	expected := `
# HELP verkada_up 1 when all the Verkada API calls in the last scrape worked, 0 when one or more failed.
# TYPE verkada_up gauge
verkada_up 0
# HELP verkada_sensor_reading Newest reading of an environmental sensor, in the units Verkada uses. The reading label says what it measures.
# TYPE verkada_sensor_reading gauge
verkada_sensor_reading{name="Server Room",reading="humidity",sensor_id="sensor-1",site="HQ"} 40
verkada_sensor_reading{name="Server Room",reading="temperature",sensor_id="sensor-1",site="HQ"} 21.5
# HELP verkada_sensor_reporting 1 when the environmental sensor sent a reading in the last 10 minutes, 0 when it did not.
# TYPE verkada_sensor_reporting gauge
verkada_sensor_reporting{name="Bathroom",sensor_id="sensor-2",site="HQ"} 0
verkada_sensor_reporting{name="Server Room",sensor_id="sensor-1",site="HQ"} 1
# HELP verkada_sensor_last_reading_timestamp_seconds Unix time of the newest reading of the environmental sensor.
# TYPE verkada_sensor_last_reading_timestamp_seconds gauge
verkada_sensor_last_reading_timestamp_seconds{name="Server Room",sensor_id="sensor-1",site="HQ"} 1.7e+09
`

	// sensor-3 does not exist, so verkada_up is 0, but the other sensors still show.
	err := testutil.CollectAndCompare(
		New(api, Options{Timeout: time.Second, SensorIDs: []string{"sensor-1", "sensor-2", "sensor-3"}}),
		strings.NewReader(expected),
		"verkada_up",
		"verkada_sensor_reading",
		"verkada_sensor_reporting",
		"verkada_sensor_last_reading_timestamp_seconds",
	)
	if err != nil {
		t.Error(err)
	}
}
