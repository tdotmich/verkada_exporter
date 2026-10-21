package verkada

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newSensorServer returns a fake Verkada API that answers /token and /environment/v1/data with sensorData.
func newSensorServer(t *testing.T, sensorData string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			fmt.Fprint(writer, `{"token":"token-1"}`)

		case "/environment/v1/data":
			if request.URL.Query().Get("device_id") != "sensor-1" {
				t.Errorf("expected device_id sensor-1, got %q", request.URL.Query().Get("device_id"))
			}

			fmt.Fprint(writer, sensorData)

		default:
			http.NotFound(writer, request)
		}
	}))
}

func TestGetLatestSensorReading(t *testing.T) {
	// The rows are not in time order, and the newest row has a null and an unknown field.
	server := newSensorServer(t, `{
		"device_id": "sensor-1",
		"device_name": "Server Room",
		"device_site_name": "HQ",
		"data": [
			{"time": 1700000060, "temperature": 22.5, "humidity": null, "motion": true, "made_up_field": 99},
			{"time": 1700000000, "temperature": 20.0, "humidity": 40}
		]
	}`)
	defer server.Close()

	reading, err := NewClient(server.URL, "test-api-key", 5*time.Second).GetLatestSensorReading(context.Background(), "sensor-1")
	if err != nil {
		t.Fatalf("GetLatestSensorReading returned an unexpected error: %v", err)
	}

	if reading.Name != "Server Room" || reading.Site != "HQ" || reading.Time != 1700000060 {
		t.Errorf("unexpected sensor details: %+v", reading)
	}

	if reading.Values["temperature"] != 22.5 {
		t.Errorf("expected the newest temperature 22.5, got %v", reading.Values["temperature"])
	}

	if reading.Values["motion"] != 1 {
		t.Errorf("expected motion true to become 1, got %v", reading.Values["motion"])
	}

	if _, exists := reading.Values["humidity"]; exists {
		t.Error("a null reading must be left out")
	}

	if _, exists := reading.Values["made_up_field"]; exists {
		t.Error("a field that is not in SensorReadingNames must be left out")
	}
}

func TestGetLatestSensorReadingNoData(t *testing.T) {
	server := newSensorServer(t, `{"device_id":"sensor-1","device_name":"Server Room","data":[]}`)
	defer server.Close()

	reading, err := NewClient(server.URL, "test-api-key", 5*time.Second).GetLatestSensorReading(context.Background(), "sensor-1")
	if err != nil {
		t.Fatalf("GetLatestSensorReading returned an unexpected error: %v", err)
	}

	if reading.Time != 0 || len(reading.Values) != 0 {
		t.Errorf("expected no reading, got %+v", reading)
	}
}

func TestListViewingStations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			fmt.Fprint(writer, `{"token":"token-1"}`)
		case "/viewing_station/v1/devices":
			fmt.Fprint(writer, `{"devices":[{"device_id":"vs-1","name":"Security Desk","last_status":"online","last_seen_at":"1700000000","claimed_serial_number":"VS123"}]}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	viewingStations, err := NewClient(server.URL, "test-api-key", 5*time.Second).ListViewingStations(context.Background())
	if err != nil {
		t.Fatalf("ListViewingStations returned an unexpected error: %v", err)
	}

	if len(viewingStations) != 1 || !viewingStations[0].IsOnline() {
		t.Fatalf("expected 1 online viewing station, got %+v", viewingStations)
	}

	lastSeen, ok := viewingStations[0].LastSeen()
	if !ok || lastSeen != 1700000000 {
		t.Errorf("expected last seen 1700000000, got %v", lastSeen)
	}
}

func TestParseNumber(t *testing.T) {
	testCases := map[string]float64{`20.5`: 20.5, `"1700000000"`: 1700000000, `true`: 1, `false`: 0}

	for raw, expected := range testCases {
		value, ok := parseNumber([]byte(raw))
		if !ok || value != expected {
			t.Errorf("parseNumber(%s): expected %v, got %v (ok=%t)", raw, expected, value, ok)
		}
	}

	for _, raw := range []string{`null`, `""`, `"text"`, ``} {
		_, ok := parseNumber([]byte(raw))
		if ok {
			t.Errorf("parseNumber(%s): expected no number", raw)
		}
	}
}
