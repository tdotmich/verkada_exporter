package collector

import (
	"context"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/tdotmich/verkada_exporter/internal/verkada"
)

// VerkadaAPI is the part of the Verkada client the collector needs.
// The tests use a fake one, so they do not need a Verkada account.
type VerkadaAPI interface {
	ListCameras(ctx context.Context) ([]verkada.Camera, error)
	ListViewingStations(ctx context.Context) ([]verkada.ViewingStation, error)
	GetLatestSensorReading(ctx context.Context, sensorID string) (*verkada.SensorReading, error)
}

// Options says what the collector gets from Verkada, in addition to the cameras.
type Options struct {
	Timeout         time.Duration
	ViewingStations bool
	SensorIDs       []string
}

// Collector gets the data from Verkada each time Prometheus scrapes /metrics.
//
// Verkada is pull: we ask the API for the data. So we use a custom collector, and
// each scrape makes fresh metrics. A device that is removed from Verkada also
// disappears from /metrics at the next scrape.
type Collector struct {
	api     VerkadaAPI
	options Options

	up             *prometheus.Desc
	scrapeDuration *prometheus.Desc

	cameraInfo     *prometheus.Desc
	cameraOnline   *prometheus.Desc
	lastOnline     *prometheus.Desc
	deviceRetain   *prometheus.Desc
	cloudRetain    *prometheus.Desc
	peopleHistory  *prometheus.Desc
	vehicleHistory *prometheus.Desc

	viewingStationInfo     *prometheus.Desc
	viewingStationOnline   *prometheus.Desc
	viewingStationLastSeen *prometheus.Desc

	sensorReading     *prometheus.Desc
	sensorReporting   *prometheus.Desc
	sensorLastReading *prometheus.Desc
}

// Every camera metric has these labels, so you can see which camera it is without a join.
var cameraLabels = []string{"camera_id", "name", "site"}

// The viewing station API gives a site ID, not a site name.
var viewingStationLabels = []string{"device_id", "name", "site_id"}

var sensorLabels = []string{"sensor_id", "name", "site"}

func New(api VerkadaAPI, options Options) *Collector {
	return &Collector{
		api:     api,
		options: options,

		up: prometheus.NewDesc(
			"verkada_up",
			"1 when all the Verkada API calls in the last scrape worked, 0 when one or more failed.",
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

		viewingStationInfo: prometheus.NewDesc(
			"verkada_viewing_station_info",
			"Information about the viewing station. The value is always 1.",
			append(viewingStationLabels, "serial", "app_version"), nil,
		),
		viewingStationOnline: prometheus.NewDesc(
			"verkada_viewing_station_online",
			"1 when the viewing station status is online, 0 when it is not.",
			viewingStationLabels, nil,
		),
		viewingStationLastSeen: prometheus.NewDesc(
			"verkada_viewing_station_last_seen_timestamp_seconds",
			"Unix time when the viewing station was last online.",
			viewingStationLabels, nil,
		),

		sensorReading: prometheus.NewDesc(
			"verkada_sensor_reading",
			"Newest reading of an environmental sensor, in the units Verkada uses. The reading label says what it measures.",
			append(sensorLabels, "reading"), nil,
		),
		sensorReporting: prometheus.NewDesc(
			"verkada_sensor_reporting",
			"1 when the environmental sensor sent a reading in the last 10 minutes, 0 when it did not.",
			sensorLabels, nil,
		),
		sensorLastReading: prometheus.NewDesc(
			"verkada_sensor_last_reading_timestamp_seconds",
			"Unix time of the newest reading of the environmental sensor.",
			sensorLabels, nil,
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
	descriptions <- collector.viewingStationInfo
	descriptions <- collector.viewingStationOnline
	descriptions <- collector.viewingStationLastSeen
	descriptions <- collector.sensorReading
	descriptions <- collector.sensorReporting
	descriptions <- collector.sensorLastReading
}

// Collect runs on each scrape. It gets the cameras, then the viewing stations and the
// sensors if they are turned on. If one part fails, the other parts still show, and
// verkada_up is 0 so you can alert on it.
func (collector *Collector) Collect(metrics chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), collector.options.Timeout)
	defer cancel()

	startTime := time.Now()
	allWorked := true

	err := collector.collectCameras(ctx, metrics)
	if err != nil {
		log.Printf("Verkada scrape failed: %v", err)
		allWorked = false
	}

	if collector.options.ViewingStations {
		err = collector.collectViewingStations(ctx, metrics)
		if err != nil {
			log.Printf("Verkada scrape failed: %v", err)
			allWorked = false
		}
	}

	for _, sensorID := range collector.options.SensorIDs {
		err = collector.collectSensor(ctx, metrics, sensorID)
		if err != nil {
			log.Printf("Verkada scrape failed: %v", err)
			allWorked = false
		}
	}

	metrics <- prometheus.MustNewConstMetric(
		collector.scrapeDuration, prometheus.GaugeValue, time.Since(startTime).Seconds(),
	)

	metrics <- prometheus.MustNewConstMetric(collector.up, prometheus.GaugeValue, boolToFloat(allWorked))
}

func (collector *Collector) collectCameras(ctx context.Context, metrics chan<- prometheus.Metric) error {
	cameras, err := collector.api.ListCameras(ctx)
	if err != nil {
		return err
	}

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

	return nil
}

func (collector *Collector) collectViewingStations(ctx context.Context, metrics chan<- prometheus.Metric) error {
	viewingStations, err := collector.api.ListViewingStations(ctx)
	if err != nil {
		return err
	}

	for _, viewingStation := range viewingStations {
		labels := []string{viewingStation.DeviceID, viewingStation.Name, viewingStation.SiteID}

		metrics <- prometheus.MustNewConstMetric(
			collector.viewingStationInfo, prometheus.GaugeValue, 1,
			append(labels, viewingStation.Serial, viewingStation.AppVersion)...,
		)

		metrics <- prometheus.MustNewConstMetric(
			collector.viewingStationOnline, prometheus.GaugeValue, boolToFloat(viewingStation.IsOnline()), labels...,
		)

		lastSeen, ok := viewingStation.LastSeen()
		if ok && lastSeen > 0 {
			metrics <- prometheus.MustNewConstMetric(
				collector.viewingStationLastSeen, prometheus.GaugeValue, lastSeen, labels...,
			)
		}
	}

	return nil
}

func (collector *Collector) collectSensor(ctx context.Context, metrics chan<- prometheus.Metric, sensorID string) error {
	reading, err := collector.api.GetLatestSensorReading(ctx, sensorID)
	if err != nil {
		return err
	}

	labels := []string{reading.SensorID, reading.Name, reading.Site}

	// No time means the sensor sent nothing in the last 10 minutes. We show no values,
	// so old values are never shown as if they were new.
	reporting := reading.Time > 0

	metrics <- prometheus.MustNewConstMetric(
		collector.sensorReporting, prometheus.GaugeValue, boolToFloat(reporting), labels...,
	)

	if !reporting {
		return nil
	}

	metrics <- prometheus.MustNewConstMetric(
		collector.sensorLastReading, prometheus.GaugeValue, reading.Time, labels...,
	)

	for readingName, value := range reading.Values {
		metrics <- prometheus.MustNewConstMetric(
			collector.sensorReading, prometheus.GaugeValue, value,
			append(labels, readingName)...,
		)
	}

	return nil
}

func boolToFloat(value bool) float64 {
	if value {
		return 1
	}

	return 0
}
