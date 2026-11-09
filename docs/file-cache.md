# File cache and preservation

Each connection offers **Use local file cache**, enabled by default. Disabling
it selects rclone VFS `off`; it does not disable memory buffers or directory
metadata caching. Direct access supports fewer file operations, depends on the
network, and may be incompatible with editing applications. There is no automatic
fallback between these modes. Change the preference only after disconnecting and
resolving pending files. Settings format 7 and portable format 3 retain this
preference; previous formats remain readable with caching enabled by default.

Cache inspection continues to reject unpaired data and invalid metadata.
**Preserve and disconnect** is a separate explicit action. It checks live writers
and queued uploads, requests normal unmount, verifies mount removal, and requests
graceful worker shutdown without forced termination. With the worker stopped,
it copies the connection's data and metadata into the profile's `recovery` folder,
verifies SHA-256 hashes, and commits a manifest. Originals remain untouched.
Cache copies may be sparse or incomplete; retain and review their metadata
before restoring file contents. Hash verification proves the copy matches the
local bytes, not that it represents a complete remote file.

An incomplete manifest blocks automatic connection startup and permits retry.
A verified preserved bundle permits application exit while recovery remains
pending. Reconnection and destructive connection changes remain blocked. No
recovered file is automatically uploaded, renamed, or discarded. The UI shows
the recovery directory; manual inspection is required before resolving files.

File replacements use a durable journal outside the active VFS data tree.
Before moving data or metadata, the engine writes and verifies copies of the
source and existing destination. It records preparation, data-move,
metadata-move and committed stages. Failed or interrupted operations retain
their copies and block cache reuse on startup. Successful operations remove
only their own journal. Two filesystem moves are not an atomic transaction;
interrupted operations require explicit preservation rather than automatic
remote rollback. Journaling temporarily needs additional local disk space and
adds local I/O to file replacements.

Inspection distinguishes missing metadata, missing pending data, invalid
metadata, access failures, changing inventories and interrupted replacements.
Details show relative filenames and at most 20 issues. A changing scan is retried
once; unresolved state is never represented as zero pending uploads.

After reviewing the bundle, **Start with a fresh cache** is a separate confirmed
action. It verifies the preserved hashes and checks that all remaining original
files still match their copies. Only then does it reset the active cache and
release the connection. Preserved copies remain available, and nothing is
uploaded automatically. If reset is interrupted, its durable marker continues
to block startup and the action can be retried.

Validation uses disposable fixtures and a loopback file service. It does not
establish universal application compatibility, cloud revision integrity,
physical power-loss durability, or a clean-VM installation. Never use personal
files as disposable validation data.

Cached reads validate the backing entry against the remote object before checking
EOF. Before validation, clean handles report the known remote size; unvalidated
local bytes cannot turn a nonempty remote file into an apparently empty file.
Dirty local entries retain their size and are not replaced by remote metadata.

On Windows, replacement moves an existing cache destination into the operation's
private journal before installing the replacement. Open readers keep their
previous file handle. Retired cache entries cannot publish metadata or uploads
under the replacement's name. Completed journals can defer cleanup until those
readers close and are not reported as interrupted operations.

Application-created temporary filenames are ordinary files in the mounted
namespace, distinct from the local cache journal. They may reach the remote
service while a save is in progress. A successful replacement must finish the
rename; failed-save remnants are preserved rather than excluded or deleted by
filename patterns.
