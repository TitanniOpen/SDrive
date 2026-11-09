//go:build windows

package integration

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sdrive/internal/core"
	"testing"
)

func TestMountedFileReplacementWithAndWithoutCache(t *testing.T) {
	requireNativeMount(t)
	for _, disabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("cacheDisabled=%t", disabled), func(t *testing.T) {
			f := newIntegrationFixture(t, true)
			s, _ := openIntegrationService(t, f.profile, nil)
			c := configureIntegration(t, s, f.url, f.letter)
			c.DisableFileCache = disabled
			if err := s.UpdateConnection(c); err != nil {
				t.Fatal(err)
			}
			isolatedConnectionWorkers(t, s, f.profile)
			if err := s.Mount(c.ID); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Unmount(c.ID) })
			point := c.Letter + `:\`
			var contents []byte
			for version := 0; version < 3; version++ {
				contents = []byte(fmt.Sprintf("disposable replacement version %d", version))
				temp := filepath.Join(point, "document.tmp")
				if err := os.WriteFile(temp, contents, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(temp, filepath.Join(point, "document.txt")); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Unmount(c.ID); err != nil {
				t.Fatal(err)
			}
			sum := fmt.Sprintf("%x", sha256.Sum256(contents))
			assertIntegrationFile(t, filepath.Join(f.destination, "library", "document.txt"), sum)
			if err := s.Mount(c.ID); err != nil {
				t.Fatal(err)
			}
			assertIntegrationFile(t, filepath.Join(point, "document.txt"), sum)
			if err := s.Unmount(c.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMountedCachePreservationKeepsUnpairedData(t *testing.T) {
	requireNativeMount(t)
	f := newIntegrationFixture(t, true)
	s, _ := openIntegrationService(t, f.profile, nil)
	c := configureIntegration(t, s, f.url, f.letter)
	isolatedConnectionWorkers(t, s, f.profile)
	if err := s.Mount(c.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Unmount(c.ID) })
	original := filepath.Join(core.CachePath(s), "vfs", "sd_"+c.Remote, c.Path, "orphan.tmp")
	if err := os.MkdirAll(filepath.Dir(original), 0700); err != nil {
		t.Fatal(err)
	}
	contents := []byte("disposable unpaired contents")
	if err := os.WriteFile(original, contents, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.PreserveAndDisconnect(c.ID); err != nil {
		t.Fatal(err)
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(contents))
	assertIntegrationFile(t, original, sum)
	assertIntegrationFile(t, filepath.Join(f.profile, "recovery", c.Remote, "vfs", c.Path, "orphan.tmp"), sum)
	if err := s.Mount(c.ID); err == nil {
		t.Fatal("recovery bypassed")
	}
	if err := s.Exit(); err != nil {
		t.Fatal(err)
	}
}

func TestMountedCachedEmptyFileRefreshesAfterRemoteChange(t *testing.T) {
	requireNativeMount(t)
	f := newIntegrationFixture(t, true)
	s, _ := openIntegrationService(t, f.profile, nil)
	c := configureIntegration(t, s, f.url, f.letter)
	isolatedConnectionWorkers(t, s, f.profile)
	if err := s.Mount(c.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Unmount(c.ID) })
	point := filepath.Join(c.Letter+`:\`, "document.txt")
	if err := os.WriteFile(point, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Unmount(c.ID); err != nil {
		t.Fatal(err)
	}
	updated := []byte("updated disposable contents after reconnect")
	if err := os.WriteFile(filepath.Join(f.destination, "library", "document.txt"), updated, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Mount(c.ID); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(point)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() != int64(len(updated)) {
		t.Fatalf("stale size: %v %v", info, err)
	}
	contents, err := io.ReadAll(file)
	if err != nil || !bytes.Equal(contents, updated) {
		t.Fatalf("cached read did not refresh remote contents: length=%d error=%v", len(contents), err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Unmount(c.ID); err != nil {
		t.Fatal(err)
	}
}

func TestMountedCachedReplacementAfterReadingDestination(t *testing.T) {
	requireNativeMount(t)
	f := newIntegrationFixture(t, true)
	s, _ := openIntegrationService(t, f.profile, nil)
	c := configureIntegration(t, s, f.url, f.letter)
	isolatedConnectionWorkers(t, s, f.profile)
	if err := s.Mount(c.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Unmount(c.ID) })
	point := c.Letter + `:\`
	original := filepath.Join(point, "document.txt")
	if err := os.WriteFile(original, []byte("original disposable contents"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Unmount(c.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Mount(c.ID); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(original)
	if err != nil || string(before) != "original disposable contents" {
		t.Fatalf("initial read: %v", err)
	}
	// Closing the client reader leaves the engine's default cached handle alive.
	temp := filepath.Join(point, "document.tmp")
	replacement := []byte("new disposable replacement contents")
	if err := os.WriteFile(temp, replacement, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temp, original); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(original)
	if err != nil || !bytes.Equal(current, replacement) {
		t.Fatalf("replacement damaged after previous reader closed: %v", err)
	}
	if err := s.Unmount(c.ID); err != nil {
		t.Fatal(err)
	}
	assertIntegrationFile(t, filepath.Join(f.destination, "library", "document.txt"), fmt.Sprintf("%x", sha256.Sum256(replacement)))
	if _, err := os.Stat(filepath.Join(f.destination, "library", "document.tmp")); !os.IsNotExist(err) {
		t.Fatal("temporary upload remained after completed replacement")
	}
}
