package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

type preservedFile struct {
	Path string
	Hash string
}
type cachePreservation struct {
	Stage string
	Files []preservedFile
}

func (s *Service) preservedCachePath(c Connection) string {
	return filepath.Join(s.dir, "recovery", c.Remote)
}
func (s *Service) hasPreservedCache(c Connection) bool {
	b, err := os.ReadFile(filepath.Join(s.preservedCachePath(c), "manifest.json"))
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		return true
	}
	var manifest cachePreservation
	return json.Unmarshal(b, &manifest) != nil || manifest.Stage != "resolved"
}

func fileHash(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	_, e = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), e
}
func (s *Service) verifiedPreservedCache(c Connection) bool {
	if s.mounted[c.ID] {
		return false
	}
	if rpc := s.workers[c.Remote]; rpc != nil && rpc.Alive() {
		return false
	}
	root := s.preservedCachePath(c)
	b, e := os.ReadFile(filepath.Join(root, "manifest.json"))
	if e != nil {
		return false
	}
	var m cachePreservation
	if json.Unmarshal(b, &m) != nil || (m.Stage != "preserved" && m.Stage != "resolving") || m.Files == nil {
		return false
	}
	for _, f := range m.Files {
		if !filepath.IsLocal(f.Path) {
			return false
		}
		h, e := fileHash(filepath.Join(root, f.Path))
		if e != nil || h != f.Hash {
			return false
		}
	}
	return true
}

// PreserveAndDisconnect preserves unknown cache contents without declaring them uploaded.
func (s *Service) PreserveAndDisconnect(id string) error {
	defer s.reconcileDesktopAppearance()
	if err := s.accessAllowed(); err != nil {
		return err
	}
	s.mu.Lock()
	i := s.index(id)
	if i < 0 || s.busy[id] || s.exiting {
		s.mu.Unlock()
		return fmt.Errorf("connection unavailable or busy")
	}
	c := s.store.Connections[i]
	rpc := s.connectionRPC(c)
	mounted := s.mounted[id]
	if mounted && rpc == s.rpc {
		s.mu.Unlock()
		return fmt.Errorf("An isolated connection engine is required to preserve files.")
	}
	s.store.Connections[i].Recovery = true
	if err := saveStore(s.dir, s.store); err != nil {
		s.mu.Unlock()
		return err
	}
	s.busy[id] = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.busy[id] = false; delete(s.inspections, id); s.snapshotID++; s.mu.Unlock() }()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	if mounted {
		var status struct {
			ActiveWriters *int
			DiskCache     *struct {
				UploadsInProgress *int
				UploadsQueued     *int
			}
		}
		if err := rpc.Call(ctx, "vfs/stats", map[string]string{"fs": remote(c)}, &status); err != nil {
			return err
		}
		if status.ActiveWriters == nil || *status.ActiveWriters != 0 {
			return fmt.Errorf("Close open files and wait for uploads before preserving the cache.")
		}
		if !c.DisableFileCache && (status.DiskCache == nil || status.DiskCache.UploadsInProgress == nil || status.DiskCache.UploadsQueued == nil || *status.DiskCache.UploadsInProgress != 0 || *status.DiskCache.UploadsQueued != 0) {
			return fmt.Errorf("Close open files and wait for uploads before preserving the cache.")
		}
	}
	root := s.preservedCachePath(c)
	for _, path := range []string{s.dir, filepath.Join(s.dir, "recovery"), root, filepath.Join(s.dataPath(), "vfs"), filepath.Join(s.dataPath(), "vfsMeta"), filepath.Join(s.dataPath(), "sdrive-renames")} {
		info, e := os.Lstat(path)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("Unexpected recovery directory; original files were preserved.")
		}
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	if b, e := os.ReadFile(filepath.Join(root, "manifest.json")); e == nil {
		var previous cachePreservation
		if json.Unmarshal(b, &previous) == nil && previous.Stage == "resolved" {
			source, e := filepath.Abs(root)
			if e != nil {
				return e
			}
			archive := source + "-" + time.Now().UTC().Format("20060102T150405.000000000")
			parent, e := filepath.Abs(filepath.Join(s.dir, "recovery"))
			if e != nil {
				return e
			}
			if filepath.Dir(source) != parent || filepath.Dir(archive) != parent {
				return fmt.Errorf("unexpected recovery archive path")
			}
			if e = os.Rename(source, archive); e != nil {
				return e
			}
			if e = os.Mkdir(root, 0700); e != nil {
				return e
			}
		}
	}
	manifest := cachePreservation{Stage: "preparing", Files: []preservedFile{}}
	save := func() error {
		b, e := json.Marshal(manifest)
		if e != nil {
			return e
		}
		return atomicWrite(filepath.Join(root, "manifest.json"), b)
	}
	if err := save(); err != nil {
		return err
	}
	if mounted {
		if err := callRPC(ctx, rpc, "mount/unmount", map[string]string{"mountPoint": mountPoint(c)}, nil); err != nil {
			return err
		}
	}
	if mounted {
		mounts, err := readMounts(ctx, rpc)
		if err != nil {
			return err
		}
		for _, m := range mounts {
			if mountKey(m.MountPoint) == mountKey(mountPoint(c)) {
				return fmt.Errorf("Could not confirm drive disconnection; original files were preserved.")
			}
		}
	}
	if rpc != nil && rpc != s.rpc && rpc.Alive() {
		closer, ok := rpc.(interface{ CloseGracefully(context.Context) error })
		if !ok {
			return fmt.Errorf("Could not confirm connection shutdown; preserve the cache.")
		}
		if err := closer.CloseGracefully(ctx); err != nil {
			return err
		}
		if rpc.Alive() {
			return fmt.Errorf("Connection engine is still running; preserve the cache.")
		}
	}
	s.mu.Lock()
	delete(s.mounted, id)
	s.stopConnection(c)
	s.mu.Unlock()
	manifest.Stage = "stopped"
	if err := save(); err != nil {
		return err
	}
	if err := s.cacheReady(); err != nil {
		return err
	}
	if err := s.copyPreservedCache(c, root, &manifest, copyVerifiedRecoveryFile); err != nil {
		return err
	}
	manifest.Stage = "preserved"
	return save()
}

func copyVerifiedRecoveryFile(path, dest string) (string, error) {
	in, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer in.Close()
	out, e := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return "", e
	}
	_, e = io.Copy(out, in)
	if e == nil {
		e = out.Sync()
	}
	closeErr := out.Close()
	if e != nil {
		return "", e
	}
	if closeErr != nil {
		return "", closeErr
	}
	original, e := fileHash(path)
	if e != nil {
		return "", e
	}
	copied, e := fileHash(dest)
	if e != nil {
		return "", e
	}
	if original != copied {
		return "", fmt.Errorf("Recovery copy verification failed; original files were preserved.")
	}
	return copied, nil
}
func (s *Service) copyPreservedCache(c Connection, root string, manifest *cachePreservation, copyFile func(string, string) (string, error)) error {
	for _, kind := range []string{"vfs", "vfsMeta", "sdrive-renames"} {
		source := filepath.Join(s.dataPath(), kind, "sd_"+c.Remote)
		err := inspectFiles(context.Background(), source, func(path string, _ fs.FileInfo) error {
			rel, e := filepath.Rel(source, path)
			if e != nil {
				return e
			}
			destRel := filepath.Join(kind, rel)
			dest := filepath.Join(root, destRel)
			if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
				return e
			}
			copied, e := copyFile(path, dest)
			if e != nil {
				return e
			}
			manifest.Files = append(manifest.Files, preservedFile{Path: destRel, Hash: copied})
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// ResolveCacheRecovery resets only bytes already verified in the retained recovery bundle.
func (s *Service) ResolveCacheRecovery(id string) error {
	if err := s.accessAllowed(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.index(id)
	if i < 0 || s.busy[id] || s.exiting {
		return fmt.Errorf("connection unavailable or busy")
	}
	c := s.store.Connections[i]
	if !s.hasPreservedCache(c) || !s.verifiedPreservedCache(c) {
		return fmt.Errorf("Preserve the files successfully before starting with a fresh cache.")
	}
	if err := s.cacheReady(); err != nil {
		return err
	}
	root := s.preservedCachePath(c)
	path := filepath.Join(root, "manifest.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var manifest cachePreservation
	if err = json.Unmarshal(b, &manifest); err != nil {
		return err
	}
	hashes := map[string]string{}
	for _, file := range manifest.Files {
		hashes[file.Path] = file.Hash
	}
	for _, kind := range []string{"vfs", "vfsMeta", "sdrive-renames"} {
		source := filepath.Join(s.dataPath(), kind, "sd_"+c.Remote)
		if err = inspectFiles(context.Background(), source, func(path string, _ fs.FileInfo) error {
			rel, e := filepath.Rel(source, path)
			if e != nil {
				return e
			}
			hash, e := fileHash(path)
			if e != nil {
				return e
			}
			if hashes[filepath.Join(kind, rel)] != hash {
				return fmt.Errorf("The original cache changed. Preserve it again before starting fresh.")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	save := func(stage string) error {
		manifest.Stage = stage
		b, e := json.Marshal(manifest)
		if e != nil {
			return e
		}
		return atomicWrite(path, b)
	}
	if err = save("resolving"); err != nil {
		return err
	}
	for _, kind := range []string{"vfs", "vfsMeta", "sdrive-renames"} {
		parent, e := filepath.Abs(filepath.Join(s.dataPath(), kind))
		if e != nil {
			return e
		}
		target, e := filepath.Abs(filepath.Join(parent, "sd_"+c.Remote))
		if e != nil {
			return e
		}
		if filepath.Dir(target) != parent {
			return fmt.Errorf("unexpected recovery cleanup path")
		}
		if err = os.RemoveAll(target); err != nil {
			return err
		}
	}
	s.store.Connections[i].Recovery = false
	if err = saveStore(s.dir, s.store); err != nil {
		s.store.Connections[i].Recovery = true
		return err
	}
	if err = save("resolved"); err != nil {
		return err
	}
	delete(s.inspections, id)
	delete(s.errors, id)
	delete(s.localizedErrors, id)
	s.snapshotID++
	return nil
}
