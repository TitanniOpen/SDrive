# Cache recovery design

Cache inspection must distinguish missing metadata, invalid metadata, missing
pending data, access failures and inventories changing during a scan. Unknown
state blocks normal disconnect and never grants permission to discard a file.
Temporary-looking names receive exactly the same protection as other files.

Cache replacements use a durable rename journal with verified pre-operation
copies. Interrupted operations block reuse of the cache and require explicit
preservation. Remote moves are never automatically reversed. See
[file-cache.md](file-cache.md) for the delivered behavior and limitations.
