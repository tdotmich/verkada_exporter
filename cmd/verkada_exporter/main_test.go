package main

import (
	"slices"
	"testing"
)

func TestParseSensorIDs(t *testing.T) {
	sensorIDs := parseSensorIDs(" sensor-1, sensor-2,,sensor-1 ")

	expected := []string{"sensor-1", "sensor-2"}
	if !slices.Equal(sensorIDs, expected) {
		t.Errorf("expected %v, got %v", expected, sensorIDs)
	}

	if len(parseSensorIDs("")) != 0 {
		t.Error("expected no sensor IDs for an empty flag")
	}
}
