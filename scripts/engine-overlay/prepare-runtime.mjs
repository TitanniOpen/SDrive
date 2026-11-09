import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const source = path.dirname(fileURLToPath(import.meta.url));

export function prepareRuntimeOverlay(rcloneDir, output) {
  const replace = {};
  mkdirSync(output, { recursive: true });
  const patchedSources = new Map();
  function patch(relative, changes) {
    let text = patchedSources.get(relative) ?? readFileSync(path.join(rcloneDir, relative), "utf8").replaceAll("\r\n", "\n");
    for (const [before, after] of changes) {
      if (text.split(before).length !== 2) throw Error(`Engine runtime hook changed: ${relative}: ${before}`);
      text = text.replace(before, after);
    }
    const target = path.join(output, relative.replaceAll("/", "_"));
    patchedSources.set(relative, text);
    writeFileSync(target, text);
    replace[path.join(rcloneDir, relative)] = target;
  }
  patch("vfs/read_write.go", [[
    'if off >= fh._size() {\n\t\treturn n, io.EOF\n\t}\n\tif err = fh.openPending(); err != nil {\n\t\treturn n, err\n\t}',
    'if err = fh.openPending(); err != nil {\n\t\treturn n, err\n\t}\n if off >= fh._size() { return n, io.EOF }',
  ], [
    'func (fh *RWFileHandle) _size() int64 {',
    'func (fh *RWFileHandle) _size() int64 {\n if !fh.opened && !fh.item.IsDirty() { if o := fh.file.getObject(); o != nil { size := o.Size(); if size >= 0 { fh.file.setSize(size); return size } } }',
  ]]);
  patch("vfs/vfs.go", [[
    'out["inUse"] = vfs.inUse.Load()',
    'out["inUse"] = vfs.inUse.Load()\n out["activeWriters"] = vfs.root.countActiveWriters()',
  ]]);
  patch("vfs/vfscache/cache.go", [[
    'root       string',
    'renameMu sync.Mutex\n renameRoot string\n root       string',
  ], [
    'root:       dataOSPath,',
    'root:       dataOSPath,\n renameRoot: filepath.Join(parentOSPath,"sdrive-renames",relativeDirOSPath),',
  ], [
    '// load in the cache and metadata off disk',
    'if err := pendingRenameJournal(filepath.Join(parentOSPath,"sdrive-renames",relativeDirOSPath)); err != nil { return nil, err }\n // load in the cache and metadata off disk',
  ]]);
  patch("vfs/vfscache/cache.go", [[
    'func (c *Cache) Rename(name string, newName string, newObj fs.Object) (err error) {',
    'func (c *Cache) Rename(name string, newName string, newObj fs.Object) (err error) {\n c.renameMu.Lock(); defer c.renameMu.Unlock()',
  ], [
    'err = item.rename(name, newName, newObj)',
    'err = c.renameReplacingItem(item,name,newName,newObj)',
  ]]);
  patch("vfs/vfscache/item.go", [[
    'type Item struct {',
    'type Item struct {\n retired bool',
  ], [
    'func (item *Item) _store(ctx context.Context, storeFn StoreFn) (err error) {',
    'func (item *Item) _store(ctx context.Context, storeFn StoreFn) (err error) {\n if item.retired {return nil}',
  ], [
    'func (item *Item) _checkObject(o fs.Object) error {',
    'func (item *Item) _checkObject(o fs.Object) error {\n if item.retired {return os.ErrNotExist}',
  ], [
    'func (item *Item) _save() (err error) {',
    'func (item *Item) _save() (err error) {\n if item.retired { return nil }',
  ], [
    'func (item *Item) _exists() bool {',
    'func (item *Item) _exists() bool {\n if item.retired { return false }',
  ], [
    'func (item *Item) _removeMeta(reason string) {',
    'func (item *Item) _removeMeta(reason string) {\n if item.retired { return }',
  ], [
    'func (item *Item) _removeFile(reason string) {',
    'func (item *Item) _removeFile(reason string) {\n if item.retired { return }',
  ], [
    'if gracePeriod > 0 && !item.info.Dirty {',
    'if gracePeriod > 0 && !item.info.Dirty && !item.retired {',
  ], [
    'func (item *Item) _actualClose(storeFn StoreFn, syncWriteBack bool) (err error) {',
    'func (item *Item) _actualClose(storeFn StoreFn, syncWriteBack bool) (err error) {\n if item.retired { item.info.Dirty=false; item.modified=false }',
  ], [
    '// Set internal state\n\titem.name = newName\n\titem.o = newObj\n\titem._updateFingerprint()',
    'renameOperation, journalErr := item.c.beginRenameJournal(name,newName)\n if journalErr != nil { item.mu.Unlock(); if downloaders != nil { _ = downloaders.Close(nil) }; return journalErr }\n defer func(){ if e:=renameOperation.finish(err==nil); e!=nil && err==nil {err=e} }()\n // Commit the identity only after both cache moves succeed.',
  ], [
    'item.mu.Unlock()\n\n\t// close downloader and cancel writebacks with mutex unlocked\n\tif downloaders != nil',
    'if err == nil { item.name = newName; item.o = newObj; item._updateFingerprint() }\n item.mu.Unlock()\n\n\t// close downloader and cancel writebacks with mutex unlocked\n\tif downloaders != nil',
  ], [
    'item.c.writeback.Rename(id, newName)',
    'if err == nil { item.c.writeback.Rename(id, newName) }',
  ], [
    'err = rename(item.c.toOSPath(name), item.c.toOSPath(newName)) // No locking in Cache',
    'err = renameOperation.move(item.c.toOSPath(name), item.c.toOSPath(newName),"data") // No locking in Cache\n if err == nil { err=renameOperation.checkpoint("data_moved") }; if err != nil { item.mu.Unlock(); if downloaders != nil { _ = downloaders.Close(nil) }; return err }',
  ], [
    'err2 := rename(item.c.toOSPathMeta(name), item.c.toOSPathMeta(newName))',
    'err2 := renameOperation.move(item.c.toOSPathMeta(name), item.c.toOSPathMeta(newName),"metadata")\n if err2 == nil {err2=renameOperation.checkpoint("metadata_moved")}',
  ]]);
  patch("vfs/file.go", [[
    'fs.Infof(f.Path(), "File.Rename failed in Cache: %v", err)',
    'fs.Errorf(f.Path(), "File.Rename failed in Cache: %v", err)\n return err',
  ]]);
  patch("fs/fshttp/dialer.go", [
    ["n, err = c.Conn.Read(b)", "n, err = c.Conn.Read(b)\n\taccounting.RecordNetworkBytes(false, n)"],
    ["n, err = c.Conn.Write(b)", "n, err = c.Conn.Write(b)\n\taccounting.RecordNetworkBytes(true, n)"],
  ]);
  patch("fs/accounting/accounting.go", [
    ["type Account struct {", "type Account struct {\n\tnetworkDirection int"],
    ["func (acc *Account) accountReadN(n int64) {", "func (acc *Account) accountReadN(n int64) {\n\trecordFileBytes(acc.networkDirection, n)"],
  ]);
  patch("fs/accounting/transfer.go", [[
    "tr.acc = newAccountSizeName(ctx, tr.stats, in, tr.size, tr.remote)",
    `tr.acc = newAccountSizeName(ctx, tr.stats, in, tr.size, tr.remote)
        if !tr.checking && tr.srcFs != nil {
            if !tr.srcFs.Features().IsLocal && (tr.dstFs == nil || tr.dstFs.Features().IsLocal) {
                tr.acc.networkDirection = 2
            } else if tr.srcFs.Features().IsLocal && tr.dstFs != nil && !tr.dstFs.Features().IsLocal {
                tr.acc.networkDirection = 1
            }
        }`,
  ]]);
  for (const name of ["network", "network_test"]) {
    replace[path.join(rcloneDir, "fs/accounting", `${name}.go`)] = path.join(source, `${name}.go.in`);
  }
  replace[path.join(rcloneDir, "fs/fshttp/network_test.go")] = path.join(source, "network_transport_test.go.in");
  replace[path.join(rcloneDir, "vfs/vfscache/cache_rename.go")] = path.join(source, "cache_rename.go.in");
  replace[path.join(rcloneDir, "vfs/vfscache/cache_rename_test.go")] = path.join(source, "cache_rename_test.go.in");
  replace[path.join(rcloneDir, "vfs/cache_read_test.go")] = path.join(source, "cache_read_test.go.in");
  return replace;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.stdout.write(JSON.stringify(prepareRuntimeOverlay(process.argv[2], process.argv[3])));
}
