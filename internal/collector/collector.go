package collector

import (
	"context"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/tdotmich/verkada_exporter/internal/verkada"
)

// CameraLister is the part of the Verkada client the collector needs.
// The tests use a fake one, so they do not need a Verkada account.
type CameraLister interface {
	ListCameras(ctx context.Context) ([]verkada.Camera, error)
}

// Collector gets the cameras from Verkada each time Prometheus scrapes /metrics.
//
// Verkada is pull: we ask the API for the data. So we use a custom collector, and
// each scrape makes fresh metrics. A camera that is removed from Verkada also
// disappears from /metrics at the next scrape.
type Collector struct {
	cameraLister CameraLister
	timeout      time.Duration

	up             *prometheus.Desc
	scrapeDuration *prometheus.Desc
	cameraInfo     *prometheus.Desc
	cameraOnline   *prometheus.Desc
	lastOnline     *prometheus.Desc
	deviceRetain   *prometheus.Desc
	cloudRetain    *prometheus.Desc
	peopleHistory  *prometheus.Desc
	vehicleHistory *prometheus.Desc
}

// Every camera metric has these labels, so you can see which camera it is without a join.
var cameraLabels = []string{"camera_id", "name", "site"}

func New(cameraLister CameraLister, timeout time.Duration) *Collector {
	return &Collector{
		cameraLister: cameraLister,
		timeout:      timeout,

		up: prometheus.NewDesc(
			"verkada_up",
			"1 when the last call to the Verkada API worked, 0 when it failed.",
			nil, nil,
		),
		scrapeDuration: prometheus.NewDesc(
			"verkada_scrape_duration_seconds",
			"Time the exporter took to get the data from the Verkada API.",
			nil, nil,
		),
		cameraInfo: prometheus.NewDesc(
			"verkada_camera_info",
			"Information about the camera. The value is always 1.",
			append(cameraLabels, "model", "serial", "firmware"), nil,
		),
		cameraOnline: prometheus.NewDesc(
			"verkada_camera_online",
			"1 when the camera status is Live, 0 when it is not.",
			cameraLabels, nil,
		),
		lastOnline: prometheus.NewDesc(
			"verkada_camera_last_online_timestamp_seconds",
			"Unix time when the camera was last online.",
			cameraLabels, nil,
		),
		deviceRetain: prometheus.NewDesc(
			"verkada_camera_device_retention_days",
			"Days of video the camera keeps on the device.",
			cameraLabels, nil,
		),
		cloudRetain: prometheus.NewDesc(
			"verkada_camera_cloud_retention_days",
			"Days of video kept in the Verkada cloud.",
			cameraLabels, nil,
		),
		peopleHistory: prometheus.NewDesc(
			"verkada_camera_people_history_enabled",
			"1 when people history is on for the camera.",
			cameraLabels, nil,
		),
		vehicleHistory: prometheus.NewDesc(
			"verkada_camera_vehicle_history_enabled",
			"1 when vehicle history is on for the camera.",
			cameraLabels, nil,
		),
	}
}

// Describe sends the description of each metric to Prometheus.
func (collector *Collector) Describe(descriptions chan<- *prometheus.Desc) {
	descriptions <- collector.up
	descriptions <- collector.scrapeDuration
	descriptions <- collector.cameraInfo
	descriptions <- collector.cameraOnline
	descriptions <- collector.lastOnline
	descriptions <- collector.deviceRetain
	descriptions <- collector.cloudRetain
	descriptions <- collector.peopleHistory
	descriptions <- collector.vehicleHistory
}

// Collect runs on each scrape. It gets the cameras and sends one set of metrics for each camera.
func (collector *Collector) Collect(metrics chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), collector.timeout)
	defer cancel()

	startTime := time.Now()
	cameras, err := collector.cameraLister.ListCameras(ctx)
	scrapeSeconds := time.Since(startTime).Seconds()

	metrics <- prometheus.MustNewConstMetric(collector.scrapeDuration, prometheus.GaugeValue, scrapeSeconds)

	// If the API call fails, we still answer the scrape. verkada_up is 0, so you can alert on it.
	if err != nil {
		log.Printf("Verkada scrape failed: %v", err)

		metrics <- prometheus.MustNewConstMetric(collector.up, prometheus.GaugeValue, 0)

		return
	}

	metrics <- prometheus.MustNewConstMetric(collector.up, prometheus.GaugeValue, 1)

	for _, camera := range cameras {
		labels := []string{camera.CameraID, camera.Name, camera.Site}

		metrics <- prometheus.MustNewConstMetric(
			collector.cameraInfo, prometheus.GaugeValue, 1,
			append(labels, camera.Model, camera.Serial, camera.Firmware)...,
		)

		metrics <- prometheus.MustNewConstMetric(
			collector.cameraOnline, prometheus.GaugeValue, boolToFloat(camera.IsOnline()), labels...,
		)

		// 0 means Verkada did not give a time, so we leave the metric out.
		if camera.LastOnline > 0 {
			metrics <- prometheus.MustNewConstMetric(
				collector.lastOnline, prometheus.GaugeValue, float64(camera.LastOnline), labels...,
			)
		}

		metrics <- prometheus.MustNewConstMetric(
			collector.deviceRetain, prometheus.GaugeValue, camera.DeviceRetentionDays, labels...,
		)

		// Cloud retention is empty (null) for a camera with no cloud backup.
		if camera.CloudRetentionDays != nil {
			metrics <- prometheus.MustNewConstMetric(
				collector.cloudRetain, prometheus.GaugeValue, *camera.CloudRetentionDays, labels...,
			)
		}

		metrics <- prometheus.MustNewConstMetric(
			collector.peopleHistory, prometheus.GaugeValue, boolToFloat(camera.PeopleHistoryEnabled), labels...,
		)

		metrics <- prometheus.MustNewConstMetric(
			collector.vehicleHistory, prometheus.GaugeValue, boolToFloat(camera.VehicleHistoryEnabled), labels...,
		)
	}
}

func boolToFloat(value bool) float64 {
	if value {
		return 1
	}

	return 0
}
