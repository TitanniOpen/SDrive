package platform

import (
	"errors"
	"testing"
)

func TestDriveUsagePreservesMaskAndFailsClosed(t *testing.T) {
	used := driveUsage(1<<uint('S'-'A'), nil)
	if !used("S") || used("T") {
		t.Fatal("incorrect drive mask")
	}
	if !used("") || !used("a") || !used("AA") {
		t.Fatal("invalid letter accepted")
	}
	failed := driveUsage(0, errors.New("native query failed"))
	if !failed("D") || !failed("Z") {
		t.Fatal("query error exposed an available drive")
	}
}
