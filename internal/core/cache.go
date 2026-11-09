package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sdrive/internal/platform"
)

const metadataLimit = 8 << 20

type UploadView struct {
	Active      int `json:"active"`
	Queued      int `json:"queued"`
	CacheErrors int `json:"cacheErrors"`
}

type CacheIssue struct {
	Code string `json:"code"`
	Path string `json:"path"`
}

type CacheView struct {
	Issues       []CacheIssue `json:"issues,omitempty"`
	IssueCount   int          `json:"issueCount,omitempty"`
	RecoveryPath string       `json:"recoveryPath,omitempty"`
	CheckedAt    *time.Time   `json:"checkedAt"`
	PendingBytes *int64       `json:"pendingBytes"`
	UsedBytes    *uint64      `json:"usedBytes"`
	Error        string       `json:"error"`
}

type localCache struct {
	CacheView
	pending int
	err     error
}

type cacheInspection struct {
	connection Connection
	at         time.Time
	complete   bool
	result     localCache
}

func cacheModeMatches(raw json.RawMessage, disabled bool) bool {
	switch string(bytes.TrimSpace(raw)) {
	case "0", `"off"`:
		return disabled
	case "3", `"full"`:
		return !disabled
	}
	return false
}

func verifyFileCacheMode(ctx context.Context, rpc RPC, c Connection) error {
	var response struct {
		Opt struct{ CacheMode json.RawMessage }
	}
	if err := rpc.Call(ctx, "vfs/stats", map[string]string{"fs": remote(c)}, &response); err != nil {
		return fmt.Errorf("Could not confirm the requested file cache mode.")
	}
	if !cacheModeMatches(response.Opt.CacheMode, c.DisableFileCache) {
		return fmt.Errorf("The engine did not enable the requested file cache mode. Review the connection status.")
	}
	return nil
}

// shortcut: these fields follow rclone v1.75.2; revalidate on engine upgrades.
func readCacheStats(ctx context.Context, rpc RPC, c Connection) (*UploadView, *uint64, error) {
	var response struct {
		ActiveWriters *int `json:"activeWriters"`
		Opt           struct{ CacheMode json.RawMessage }

		DiskCache *struct {
			UploadsInProgress *int
			UploadsQueued     *int
			ErroredFiles      *int
			Files             *int
			OutOfSpace        *bool
			BytesUsed         *int64
		}
	}
	if rpc == nil {
		return nil, nil, fmt.Errorf("could not check pending uploads; engine unavailable")
	}
	if err := rpc.Call(ctx, "vfs/stats", map[string]string{"fs": remote(c)}, &response); err != nil {
		return nil, nil, fmt.Errorf("could not check pending uploads; preserve the cache")
	}
	if c.DisableFileCache {
		if !cacheModeMatches(response.Opt.CacheMode, c.DisableFileCache) || response.ActiveWriters == nil || *response.ActiveWriters < 0 {
			return nil, nil, fmt.Errorf("could not confirm direct file writes; preserve the connection")
		}
		return &UploadView{Active: *response.ActiveWriters}, nil, nil
	}
	d := response.DiskCache
	if d == nil || d.UploadsInProgress == nil || d.UploadsQueued == nil || d.ErroredFiles == nil || d.Files == nil || d.OutOfSpace == nil || d.BytesUsed == nil {
		return nil, nil, fmt.Errorf("rclone did not confirm cache state; preserve the connection")
	}
	if *d.UploadsInProgress < 0 || *d.UploadsQueued < 0 || *d.ErroredFiles < 0 || *d.Files < 0 || *d.BytesUsed < 0 || *d.UploadsInProgress > math.MaxInt-*d.UploadsQueued {
		return nil, nil, fmt.Errorf("invalid cache statistics; preserve the connection")
	}
	uploads := &UploadView{Active: *d.UploadsInProgress, Queued: *d.UploadsQueued, CacheErrors: *d.ErroredFiles}
	used := uint64(*d.BytesUsed)
	if *d.OutOfSpace {
		return uploads, &used, fmt.Errorf("cache disk is full; preserve the cache and free up space")
	}
	return uploads, &used, nil
}

// Each directory is read in batches so cancellation does not require loading its entire listing.
func inspectFiles(ctx context.Context, root string, visit func(string, fs.FileInfo) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unexpected cache directory")
	}
	dir, err := os.Open(root)
	if err != nil {
		return err
	}
	defer dir.Close()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, readErr := dir.ReadDir(128)
		if readErr != nil && readErr != io.EOF {
			return readErr
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			path := filepath.Join(root, entry.Name())
			if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
				return fmt.Errorf("unexpected cache entry")
			}
			if info.IsDir() {
				err = inspectFiles(ctx, path, visit)
			} else {
				err = visit(path, info)
			}
			if err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			return ctx.Err()
		}
	}
}

func readDirty(ctx context.Context, path string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, metadataLimit+1))
	if err != nil || len(b) > metadataLimit {
		return false, fmt.Errorf("could not read bounded cache metadata")
	}
	var info struct{ Dirty *bool }
	if err = json.Unmarshal(b, &info); err != nil || info.Dirty == nil {
		return false, fmt.Errorf("cache metadata must contain an explicit Dirty boolean")
	}
	return *info.Dirty, ctx.Err()
}

func (s *Service) inspectCache(ctx context.Context, c Connection) localCache {
	result := s.inspectCacheOnce(ctx, c)
	if errors.Is(result.err, os.ErrNotExist) && ctx.Err() == nil {
		return s.inspectCacheOnce(ctx, c)
	}
	return result
}

func (s *Service) renameJournalPending(c Connection) bool {
	root := filepath.Join(s.dataPath(), "sdrive-renames", "sd_"+c.Remote)
	found := false
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "rename-") {
			if children, e := os.ReadDir(path); e == nil && len(children) == 0 {
				return fs.SkipDir
			}
			b, e := os.ReadFile(filepath.Join(path, "manifest.json"))
			var journal struct{ Stage string }
			if e == nil && json.Unmarshal(b, &journal) == nil && journal.Stage == "committed" {
				return fs.SkipDir
			}
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() || strings.HasPrefix(entry.Name(), "rename-") {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found || err != nil
}

func (s *Service) inspectCacheOnce(ctx context.Context, c Connection) localCache {
	result := localCache{}
	if s.hasPreservedCache(c) {
		result.RecoveryPath = s.preservedCachePath(c)
	}
	add := func(code, path string, err error) {
		for _, issue := range result.Issues {
			if issue.Code == code && issue.Path == filepath.ToSlash(path) {
				return
			}
		}
		result.IssueCount++
		if len(result.Issues) < 20 {
			result.Issues = append(result.Issues, CacheIssue{Code: code, Path: filepath.ToSlash(path)})
		}
		if result.err == nil {
			result.err = err
		}
		result.Error = "Local cache files could not be verified. Preserve and disconnect to keep a recovery copy."
	}
	if err := s.cacheReady(); err != nil {
		add("access_failed", "", err)
		return result
	}
	if s.renameJournalPending(c) {
		add("interrupted_rename", "", fmt.Errorf("interrupted cache rename requires preservation"))
	}
	meta := filepath.Join(s.dataPath(), "vfsMeta", "sd_"+c.Remote)
	data := filepath.Join(s.dataPath(), "vfs", "sd_"+c.Remote)
	for _, path := range []string{s.dataPath(), filepath.Dir(meta), filepath.Dir(data), filepath.Join(s.dataPath(), "sdrive-renames")} {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			add("access_failed", "", err)
			return result
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			add("unexpected_entry", "", fmt.Errorf("unexpected cache directory"))
			return result
		}
	}
	classify := func(err error) string {
		if errors.Is(err, os.ErrNotExist) {
			return "inventory_changed"
		}
		if errors.Is(err, os.ErrPermission) {
			return "access_failed"
		}
		return "invalid_metadata"
	}
	var used uint64
	var pendingBytes int64
	usageKnown, sizeKnown := true, true
	dirtyFiles := map[string]bool{}
	seenDirty := 0
	err := inspectFiles(ctx, meta, func(path string, _ fs.FileInfo) error {
		rel, _ := filepath.Rel(meta, path)
		dirty, err := readDirty(ctx, path)
		if err != nil {
			add(classify(err), rel, err)
			return ctx.Err()
		}
		if dirty {
			result.pending++
			dirtyFiles[rel] = true
			info, err := os.Lstat(filepath.Join(data, rel))
			if err != nil {
				code := classify(err)
				if os.IsNotExist(err) {
					code = "missing_data"
				}
				add(code, rel, err)
			} else if !info.Mode().IsRegular() {
				add("unexpected_entry", rel, fmt.Errorf("unexpected cache entry"))
			}
		}
		return ctx.Err()
	})
	if err != nil {
		add(classify(err), "", err)
	}
	if ctx.Err() == nil {
		err = inspectFiles(ctx, data, func(path string, info fs.FileInfo) error {
			rel, _ := filepath.Rel(data, path)
			dirty, err := readDirty(ctx, filepath.Join(meta, rel))
			if err != nil {
				code := classify(err)
				if os.IsNotExist(err) {
					code = "missing_metadata"
				}
				add(code, rel, err)
				return ctx.Err()
			}
			if dirty && !dirtyFiles[rel] {
				result.pending++
				dirtyFiles[rel] = true
			}
			if dirtyFiles[rel] {
				seenDirty++
				if info.Size() < 0 || pendingBytes > math.MaxInt64-info.Size() {
					sizeKnown = false
				} else {
					pendingBytes += info.Size()
				}
			}
			n, err := platform.AllocatedBytes(path)
			if err != nil || used > math.MaxUint64-n {
				usageKnown = false
			} else {
				used += n
			}
			return ctx.Err()
		})
		if err != nil {
			add(classify(err), "", err)
		}
	}
	if result.err != nil {
		return result
	}
	if usageKnown {
		result.UsedBytes = &used
	}
	if sizeKnown && seenDirty == result.pending {
		result.PendingBytes = &pendingBytes
	}
	return result
}

func (s *Service) cachedPending(ctx context.Context, c Connection) (int, error) {
	result := s.inspectCache(ctx, c)
	return result.pending, result.err
}

func (s *Service) sampleCache(ctx context.Context, c Connection) localCache {
	s.mu.Lock()
	i := s.index(c.ID)
	if i < 0 || s.store.Connections[i] != c || s.busy[c.ID] {
		s.mu.Unlock()
		return localCache{err: fmt.Errorf("connection changed or busy")}
	}
	if s.inspections == nil {
		s.inspections = map[string]*cacheInspection{}
	}
	if previous := s.inspections[c.ID]; previous != nil && previous.connection == c && (!previous.complete || previous.result.err == nil && time.Since(previous.at) < 30*time.Second) {
		complete, result := previous.complete, previous.result
		s.mu.Unlock()
		if complete {
			return result
		}
		return localCache{err: fmt.Errorf("cache inspection in progress")}
	}
	if ctx.Err() != nil {
		s.mu.Unlock()
		return localCache{err: ctx.Err()}
	}
	inspection := &cacheInspection{connection: c, at: time.Now()}
	s.inspections[c.ID] = inspection
	s.mu.Unlock()
	result := s.inspectCache(ctx, c)
	result.CheckedAt = &inspection.at
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inspections[c.ID] != inspection {
		return localCache{err: fmt.Errorf("cache inspection invalidated")}
	}
	inspection.result, inspection.complete = result, true
	return result
}
