# Phase 1: imports and preserved evidence

## Contract

Files contain records. Rules interpret records into plays and the view. Import
identity is distinct from play matching. The first supported upload format is
Spotify extended streaming history (JSON or ZIP containing history JSON).

Original bytes live at `<config>/sources/sha256/<first two hex>/<sha256>`.
Back up that directory together with `koito.db`; the database alone is not a
complete backup. Paths are derived from hashes, never uploaded filenames.

The additive migration leaves existing catalog, listens, and aliases intact.
New tables keep sources, origin metadata, files, receipts, archive membership, and raw
records separate from derived interpretations. Raw records retain every field
and every occurrence, including identical rows at distinct positions. Original
bytes are authoritative for formatting. Parsed record fields are conveniences.

Re-uploading the same bytes reuses their records. Archive
members are independently content-addressed, so uploading a member separately
does not multiply records. Different exports retain separate record occurrences;
the minimal matching rule links overlapping records into plays.

## First rules

`spotify-v1` matches stored source, original timestamp and track identity (Spotify
URI when present; otherwise title/artist/album). It makes no fuzzy or cross-service
matches. It counts music records reporting `reason_end=trackdone`, following the
old importer, but retains short, skipped, nonmusic, and invalid records with an
explanation. Played milliseconds are not assumed to be a recording's duration.
These rules are internal to the first projection. Broader qualification and
matching controls belong to the later Filters work.

The compatibility projection is rebuildable from preserved records, writes to
Koito's existing catalog/listens, and tracks which listens it owns. It must not
delete legacy or externally submitted listens. Existing catalog aliases are not
rewritten by rebuilding. The old listen key is track plus whole-second timestamp;
collisions are reported rather than claimed to be independent displayed plays.

## Import lifecycle and acceptance

Uploads require an authenticated administrator and automatically use the single
local owner. Existing source labels are retained internally. Preserve
bytes before acknowledging successful ingestion. Each attempt has a receipt,
including validation failures. Parsing/record insertion is transactional; failed
attempts leave no partial record set and can be retried. Projection failure must
not discard successfully stored evidence. Bound both upload and expanded ZIP
sizes; never extract arbitrary archive paths or execute supporting files.

The Imports page must show receipt status, filenames, source, record and
play counts, errors, record payloads and explanations, original-file download,
and a rebuild action. No public endpoint exposes raw evidence.
The upload form is generic: recognize supported content regardless of the outer
filename and reject unsupported files. Keep rule details and explanatory prose
out of the page; show controls, data, and concise action results.

The remaining phases and shared vocabulary are in the
[development plan](development-plan.md).

Acceptance tests cover repeats, renamed uploads, zipped/standalone overlap,
overlapping exports, repeated rows within one file, distinct accounts, unknown
fields, skipped/nonmusic records, malformed input, failed attempts/retries,
projection rebuild, existing-data preservation, and authenticated HTTP behavior.
Real dump files are local acceptance inputs, not committed fixtures.

## Transition

Existing startup importers remain available for Last.fm, Maloja, ListenBrainz,
and Koito until their adapters are migrated. Their outputs have legacy provenance;
they must not be presented as lossless imports. The new upload workflow preserves
Spotify evidence. Production database replacement and automatic ingestion of
the user's dump are outside this development phase.

## Verification during development

- `go test ./internal/sources` and `go vet ./internal/sources`: passed on Windows.
  Includes 100 repeat uploads, ZIP/member overlap, cross-export matching,
  separate accounts, immutable raw records, malformed imports, legacy migration,
  and projection failure recovery.
- Real local Spotify file: 8,524 preserved records, 3,383 qualifying plays;
  re-upload added zero records, and repeated rebuilds returned identical counts.
- `yarn typecheck` and `yarn build`: passed.
- HTTP tests and desktop/mobile browser checks pass. The page has one Import file
  action, starts importing after selection, and requires no account field.
- Final native browser acceptance used the real 8,524-record export. Original
  bytes and record payloads matched; renamed re-upload and ZIP/standalone overlap
  added no records; rebuilding preserved 3,383 plays; unsupported input was
  rejected; the listening-history page displayed the resulting plays.
- Native Windows setup is now available through `scripts/windows.ps1`. The full
  Go suite, including engine integration tests and real libvips image encoding,
  passes without the temporary overlay. See [Windows development](windows-development.md).

Windows file handling fixes close startup-import files before moving them, create image-cache
directories with platform-aware paths, and stop the integration-test server before
database cleanup. Linux deployment checks remain separate from local development.
