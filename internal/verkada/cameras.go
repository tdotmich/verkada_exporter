package verkada

import (
	"context"
	"fmt"
	"net/url"
)

// Camera is one camera from GET /cameras/v1/devices.
type Camera struct {
	CameraID              string   `json:"camera_id"`
	Name                  string   `json:"name"`
	Site                  string   `json:"site"`
	SiteID                string   `json:"site_id"`
	Model                 string   `json:"model"`
	Serial                string   `json:"serial"`
	Status                string   `json:"status"`
	Firmware              string   `json:"firmware"`
	LastOnline            int64    `json:"last_online"`
	DeviceRetentionDays   float64  `json:"device_retention"`
	CloudRetentionDays    *float64 `json:"cloud_retention"`
	PeopleHistoryEnabled  bool     `json:"people_history_enabled"`
	VehicleHistoryEnabled bool     `json:"vehicle_history_enabled"`
}

// IsOnline is true when Verkada shows the camera as "Live".
func (camera Camera) IsOnline() bool {
	return camera.Status == "Live"
}

type devicesResponse struct {
	Cameras       []Camera `json:"cameras"`
	NextPageToken string   `json:"next_page_token"`
}

// ListCameras returns all the cameras in the organization. It reads each page
// until Verkada returns no next page token.
func (client *Client) ListCameras(ctx context.Context) ([]Camera, error) {
	var allCameras []Camera

	pageToken := ""

	for page := 1; page <= maxCameraPages; page++ {
		query := url.Values{}
		query.Set("page_size", fmt.Sprint(camerasPerPage))

		if pageToken != "" {
			query.Set("page_token", pageToken)
		}

		requestURL := client.baseURL + "/cameras/v1/devices?" + query.Encode()

		var devices devicesResponse

		err := client.getJSON(ctx, requestURL, &devices)
		if err != nil {
			return nil, fmt.Errorf("list cameras: %w", err)
		}

		allCameras = append(allCameras, devices.Cameras...)

		if devices.NextPageToken == "" {
			return allCameras, nil
		}

		pageToken = devices.NextPageToken
	}

	return nil, fmt.Errorf("list cameras: more than %d pages", maxCameraPages)
}
