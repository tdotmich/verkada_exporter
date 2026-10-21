package verkada

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ViewingStation is one viewing station from GET /viewing_station/v1/devices.
type ViewingStation struct {
	DeviceID   string          `json:"device_id"`
	Name       string          `json:"name"`
	SiteID     string          `json:"site_id"`
	Serial     string          `json:"claimed_serial_number"`
	LastStatus string          `json:"last_status"`
	AppVersion string          `json:"app_version"`
	LastSeenAt json.RawMessage `json:"last_seen_at"`
}

// IsOnline is true when Verkada shows the viewing station as "online".
func (viewingStation ViewingStation) IsOnline() bool {
	return strings.EqualFold(viewingStation.LastStatus, "online")
}

// LastSeen returns the Unix time when the viewing station was last online.
// The docs give it as a string, so it is read with parseNumber.
func (viewingStation ViewingStation) LastSeen() (float64, bool) {
	return parseNumber(viewingStation.LastSeenAt)
}

type viewingStationsResponse struct {
	Devices []ViewingStation `json:"devices"`
}

// ListViewingStations returns all the viewing stations in the organization.
// This endpoint has no pages.
func (client *Client) ListViewingStations(ctx context.Context) ([]ViewingStation, error) {
	var viewingStations viewingStationsResponse

	err := client.getJSON(ctx, client.baseURL+"/viewing_station/v1/devices", &viewingStations)
	if err != nil {
		return nil, fmt.Errorf("list viewing stations: %w", err)
	}

	return viewingStations.Devices, nil
}
