package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPreservationRetainsOrphanBytesAndBlocksReconnect(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	s.store.Connections = []Connection{c}
	data := "recoverable bytes"
	meta := cacheFixture(t, s, c, `{"Dirty":true}`, &data)
	if e := os.Remove(meta); e != nil {
		t.Fatal(e)
	}
	if e := s.PreserveAndDisconnect(c.ID); e != nil {
		t.Fatal(e)
	}
	if !s.verifiedPreservedCache(c) {
		t.Fatal("preservation not verified")
	}
	original := filepath.Join(s.dataPath(), "vfs", "sd_"+c.Remote, "nested", "file")
	b, e := os.ReadFile(original)
	if e != nil || string(b) != data {
		t.Fatal("original changed")
	}
	copy := filepath.Join(s.preservedCachePath(c), "vfs", "nested", "file")
	b, e = os.ReadFile(copy)
	if e != nil || string(b) != data {
		t.Fatal("copy missing")
	}
	if _, e = s.prepareConnection(context.Background(), c, false); e == nil {
		t.Fatal("reconnected with unresolved recovery")
	}
	if e = os.WriteFile(copy, []byte("corrupt"), 0600); e != nil {
		t.Fatal(e)
	}
	if s.verifiedPreservedCache(c) {
		t.Fatal("accepted damaged copy")
	}
}

func TestPreservationResumesAfterInterruptedCopyAndRestart(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	s.store.Connections = []Connection{c}
	content := "pending contents"
	cacheFixture(t, s, c, `{"Dirty":true}`, &content)
	blocked := filepath.Join(s.preservedCachePath(c), "vfs", "nested", "file")
	if err := os.MkdirAll(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.PreserveAndDisconnect(c.ID); err == nil {
		t.Fatal("copy failure declared preserved")
	}
	if s.verifiedPreservedCache(c) {
		t.Fatal("incomplete manifest accepted")
	}
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(s.dir, &fakeRPC{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.prepareConnection(context.Background(), c, false); err == nil {
		t.Fatal("restart bypassed recovery")
	}
	if err := restarted.PreserveAndDisconnect(c.ID); err != nil {
		t.Fatal(err)
	}
	if !restarted.verifiedPreservedCache(c) {
		t.Fatal("retry did not preserve files")
	}
}
func TestPreservationCopyFailureKeepsOriginalAndIncompleteManifest(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	content := "original"
	path := cacheFixture(t, s, c, `{"Dirty":true}`, &content)
	root := s.preservedCachePath(c)
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := cachePreservation{Stage: "stopped", Files: []preservedFile{}}
	b, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	err := s.copyPreservedCache(c, root, &manifest, func(string, string) (string, error) { return "", syscall.ENOSPC })
	if err == nil || s.verifiedPreservedCache(c) {
		t.Fatal("disk-full copy accepted")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("original metadata lost")
	}
}

func TestResolveRecoveryRequiresVerifiedUnchangedCopiesAndRetainsBundle(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	s.store.Connections = []Connection{c}
	content := "original"
	cacheFixture(t, s, c, `{"Dirty":true}`, &content)
	if err := s.PreserveAndDisconnect(c.ID); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(s.dataPath(), "vfs", "sd_"+c.Remote, "nested", "file")
	if err := os.WriteFile(original, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.ResolveCacheRecovery(c.ID); err == nil {
		t.Fatal("deleted changed originals")
	}
	if err := os.WriteFile(original, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.ResolveCacheRecovery(c.ID); err != nil {
		t.Fatal(err)
	}
	if s.hasPreservedCache(c) || s.store.Connections[0].Recovery {
		t.Fatal("resolution did not release connection")
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatal("active cache not reset")
	}
	b, err := os.ReadFile(filepath.Join(s.preservedCachePath(c), "vfs", "nested", "file"))
	if err != nil || string(b) != content {
		t.Fatal("preserved bundle removed")
	}
	restarted, err := New(s.dir, &fakeRPC{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if restarted.hasPreservedCache(c) {
		t.Fatal("resolved state did not survive restart")
	}
}

func TestNewPreservationArchivesPreviouslyResolvedBundle(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	s.store.Connections = []Connection{c}
	old := "previous bytes"
	cacheFixture(t, s, c, `{"Dirty":true}`, &old)
	if err := s.PreserveAndDisconnect(c.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ResolveCacheRecovery(c.ID); err != nil {
		t.Fatal(err)
	}
	next := "new bytes"
	cacheFixture(t, s, c, `{"Dirty":true}`, &next)
	if err := s.PreserveAndDisconnect(c.ID); err != nil {
		t.Fatal(err)
	}
	archives, err := filepath.Glob(s.preservedCachePath(c) + "-*")
	if err != nil || len(archives) != 1 {
		t.Fatal("previous recovery bundle not retained")
	}
	b, err := os.ReadFile(filepath.Join(archives[0], "vfs", "nested", "file"))
	if err != nil || string(b) != old {
		t.Fatal("previous recovery bytes changed")
	}
	if !s.verifiedPreservedCache(c) {
		t.Fatal("new recovery not verified")
	}
}
