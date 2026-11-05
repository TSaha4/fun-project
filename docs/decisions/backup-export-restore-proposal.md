# Backup, export, and restore proposal

**Status:** Proposed; cryptographic implementation awaits explicit approval.
**Scope:** Portable note export, full local backup, integrity validation, and restore into a new vault directory.

## Repository state found

- `internal/vault` is a placeholder. Vault creation, password unlock, database encryption, and encrypted attachment storage are not implemented or wired into the desktop/API/MCP compositions.
- SQLite schema is currently version 2. It stores collections, tags, notes, note-tag and note-attachment relationships, reminders/history, and attachment metadata. Notes and FTS data are plaintext.
- Attachment bytes have no production file store. The database contains attachment metadata only, so a backup cannot currently include real attachment contents or prove that a referenced attachment is present.
- There is no backup/export/restore application service or API.

## Proposed security and file design

### Encrypted backup (default)

Create a complete snapshot ZIP in a private temporary location, then stream it into the standard age v1 encrypted-file format as `*.age`. Use the maintained [`filippo.io/age`](https://pkg.go.dev/filippo.io/age) Go library with its passphrase recipient/identity. Age supports streaming encryption and uses the standardized scrypt recipient format, avoiding a new custom encryption container. RFC 9106 recommends Argon2id for new password-based designs, including a 64 MiB, 3-pass option for memory-constrained systems; age's interoperable passphrase format instead uses scrypt. If Argon2id is mandatory for backups, that requires a distinct approved container design and independent format review. [RFC 9106](https://www.rfc-editor.org/rfc/rfc9106), [age Go package](https://pkg.go.dev/filippo.io/age).

The backup password is requested through an application password prompt or secure callback; never through command-line arguments, environment variables, logs, or a file beside the backup. Use a separate backup passphrase unless a future approved vault design provides a tested key-reuse/export protocol. Losing the passphrase means the backup cannot be recovered; the product must not claim password recovery. Age streaming avoids loading a large archive into memory. Restore sets a maximum accepted scrypt work factor to limit malicious-file CPU/memory exhaustion.

The current repository has no encrypted source vault to preserve. Once vault encryption exists, snapshot plaintext only inside the unlocked application process and encrypt the complete backup before it leaves the process. Never label the current plaintext SQLite source as encrypted. Restore decrypts into a private staging directory, verifies the archive and every checksum, then installs into a newly created destination only.

### Portable exports

- **JSON:** full structured data, including attachment bytes encoded as base64 once a production attachment store exists. SHA-256 values detect accidental corruption; without encryption or a secret MAC they do not authenticate a deliberately modified export.
- **Markdown:** human-readable note files with stable IDs and front matter. This is an explicitly lossy interchange format and should say so; it is not the recovery backup.
- **ZIP:** full portable snapshot with `manifest.json`, `notes/<note-id>.md`, and `attachments/<attachment-id>.<safe-extension>`. The manifest carries collection/tag data, relationships, reminder state/history, schema and format versions, sizes, media types, and SHA-256 hashes. Paths in the manifest must be IDs or normalized relative paths, never trusted filesystem paths.
- The full backup payload is the ZIP snapshot wrapped by age. JSON and Markdown exports are plaintext by nature and must require an explicit `allowPlaintext` confirmation that names the destination and warns that deletion cannot guarantee secure erasure. No default export action may silently choose JSON/Markdown.

### Validation and restore

1. Decrypt (when applicable) into a fresh private staging directory; enforce total archive, entry-count, per-entry, and decompressed-size limits while streaming.
2. Reject absolute paths, drive letters, UNC paths, `..`, symlinks, duplicate normalized names, duplicate entity IDs, unsupported format/schema versions, malformed timestamps/types/relationships, and entries not listed by the manifest.
3. Verify manifest entry sizes and SHA-256 checksums; verify ZIP CRC and encrypted age authentication. Missing attachment bytes are fatal for a full backup, not silently ignored.
4. Validate all references before writing. Open an empty SQLite database in a newly created staging vault, apply supported migrations, import within one transaction through an application-level snapshot port, and rebuild/verify FTS indexes.
5. Close the database, fsync where supported, and atomically move the complete staging vault to a destination that did not already exist. Never overwrite or merge into the active vault. Leave the source backup and source vault untouched on every failure.

Checksums detect accidental damage, not malicious edits to an unencrypted export. Age authentication protects encrypted backup payloads. The archive's filenames and coarse size can remain visible in a standard age file; use opaque attachment IDs and generic archive entry names to reduce metadata leakage.

## Proposed implementation plan after approval

1. Implement a production attachment file store with opaque IDs, safe relative paths, atomic writes, streaming reads, and checksum verification. Existing attachment metadata alone is insufficient for ZIP/JSON attachment export.
2. Add an application snapshot interface for notes, collections, tags, relationships, reminder/history metadata, attachment metadata, and the supported schema version. Keep SQLite behind this port.
3. Implement versioned manifest and JSON/Markdown/ZIP encoders, encrypted age streaming, bounded validation, and fresh-location restore orchestration in `internal/backup`.
4. Add CLI/API entry points only after the core backup interfaces pass review; collect passphrases through secure UI/input callbacks, not flags or environment variables.
5. Test round trips, wrong passwords, corrupt/truncated archives, missing attachment bytes, duplicate IDs/entries, unsupported versions, path traversal, size limits, and failure atomicity using temporary directories/databases.
6. Document manual recovery, compatibility policy, and independent security review requirements.

## Approval needed

Before implementing encryption, approve or revise these choices:

1. Use the established age v1 streaming format with its passphrase/scrypt mode for encrypted backups (interoperable and streaming), rather than a custom Argon2id envelope. The vault itself remains a separate, still-unapproved encryption design.
2. Require an explicit user confirmation for plaintext JSON/Markdown export; encrypted ZIP backup is the default backup path. Markdown is documented as non-restorable/lossy.
3. Implement the missing attachment file store as a necessary dependency so ZIP backups actually preserve attachment bytes.
