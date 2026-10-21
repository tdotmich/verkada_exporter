package verkada

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// SensorReadingNames are the readings Verkada documents for its environmental sensors
// (SV11, SV23, SV25). A sensor only sends the readings its model supports.
// We only export names from this list, so the number of series stays fixed.
var SensorReadingNames = []string{
	"temperature",
	"humidity",
	"heat_index",
	"carbon_dioxide",
	"carbon_monoxide",
	"formaldehyde",
	"pm_1_0_0",
	"pm_2_5",
	"pm_4_0",
	"tvoc",
	"tvoc_index",
	"usa_air_quality_index",
	"vape_index",
	"noise_level",
	"barometric_pressure",
	"ambient_light",
	"motion",
	"tamper",
}

// sensorLookback is how far back we ask for readings. We only keep the newest one,
// but a short window makes sure there is at least one reading in the response.
const sensorLookback = 10 * time.Minute

// SensorReading is the newest reading of one environmental sensor.
type SensorReading struct {
	SensorID string
	Name     string
	Site     string
	Time     float64
	Values   map[string]float64
}

type sensorDataResponse struct {
	DeviceID       string                       `json:"device_id"`
	DeviceName     string                       `json:"device_name"`
	DeviceSiteName string                       `json:"device_site_name"`
	Data           []map[string]json.RawMessage `json:"data"`
}

// GetLatestSensorReading asks for the readings of the last 10 minutes, one row per
// minute, and returns the newest row. Verkada has no endpoint that lists all sensors,
// so the caller must give the sensor ID.
func (client *Client) GetLatestSensorReading(ctx context.Context, sensorID string) (*SensorReading, error) {
	endTime := time.Now()
	startTime := endTime.Add(-sensorLookback)

	query := url.Values{}
	query.Set("device_id", sensorID)
	query.Set("start_time", fmt.Sprint(startTime.Unix()))
	query.Set("end_time", fmt.Sprint(endTime.Unix()))
	query.Set("interval", "1m")
	query.Set("page_size", "200")

	requestURL := client.baseURL + "/environment/v1/data?" + query.Encode()

	var sensorData sensorDataResponse

	err := client.getJSON(ctx, requestURL, &sensorData)
	if err != nil {
		return nil, fmt.Errorf("get sensor %s data: %w", sensorID, err)
	}

	reading := &SensorReading{
		SensorID: sensorID,
		Name:     sensorData.DeviceName,
		Site:     sensorData.DeviceSiteName,
		Values:   map[string]float64{},
	}

	// The docs do not say how the rows are sorted, so we look for the newest time.
	var newestRow map[string]json.RawMessage

	for _, row := range sensorData.Data {
		rowTime, ok := parseNumber(row["time"])
		if ok && rowTime > reading.Time {
			reading.Time = rowTime
			newestRow = row
		}
	}

	// No rows means the sensor sent nothing in the last 10 minutes.
	if newestRow == nil {
		return reading, nil
	}

	for _, readingName := range SensorReadingNames {
		value, ok := parseNumber(newestRow[readingName])
		if ok {
			reading.Values[readingName] = value
		}
	}

	return reading, nil
}
