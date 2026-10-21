package collector

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/tdotmich/verkada_exporter/internal/verkada"
)

// fakeLister returns fixed cameras, or an error, instead of calling Verkada.
type fakeLister struct {
	cameras []verkada.Camera
	err     error
}

func (lister fakeLister) ListCameras(ctx context.Context) ([]verkada.Camera, error) {
	return lister.cameras, lister.err
}

func TestCollectCameraMetrics(t *testing.T) {
	cloudDays := 30.0

	lister := fakeLister{cameras: []verkada.Camera{
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
# HELP verkada_up 1 when the last call to the Verkada API worked, 0 when it failed.
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
		New(lister, time.Second),
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
	lister := fakeLister{err: errors.New("verkada API returned HTTP status 429 Too Many Requests")}

	expected := `
# HELP verkada_up 1 when the last call to the Verkada API worked, 0 when it failed.
# TYPE verkada_up gauge
verkada_up 0
`

	err := testutil.CollectAndCompare(New(lister, time.Second), strings.NewReader(expected), "verkada_up")
	if err != nil {
		t.Error(err)
	}

	// No camera metrics on a failed scrape.
	count := testutil.CollectAndCount(New(lister, time.Second), "verkada_camera_online")
	if count != 0 {
		t.Errorf("expected no camera metrics, got %d", count)
	}
}
