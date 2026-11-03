package core

import (
	"sdrive/internal/platform"
	"testing"
)

func TestSnapshotSamplesPlatformAvailabilityOnce(t *testing.T) {
	s, err := New(t.TempDir(), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	mountCalls, usageCalls := 0, 0
	s.mountAvailable = func() bool { mountCalls++; return true }
	s.driveUsage = func() func(string) bool { usageCalls++; return func(letter string) bool { return letter == "S" } }
	view := s.Snapshot()
	if mountCalls != 1 || !view.MountAvailable {
		t.Fatalf("mount queries: %d", mountCalls)
	}
	if platform.OS == "windows" {
		if usageCalls != 1 {
			t.Fatalf("drive mask queries: %d", usageCalls)
		}
		for _, letter := range view.Letters {
			if letter == "S" {
				t.Fatal("occupied drive offered")
			}
		}
		if len(view.Letters) != 22 {
			t.Fatalf("available letters: %v", view.Letters)
		}
	} else if usageCalls != 0 {
		t.Fatal("Windows drive mask queried on Linux")
	}
}
