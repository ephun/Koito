# Phase 6 closing audit

Scope: Phase 6 and the requirements shared by every phase in `development-plan.md`.

## Requirement evidence

| Requirement | Implementation and authoritative checks |
| --- | --- |
| Consistent Navidrome snapshots with WAL | `sqlite_reader.go`, `sqlite_wal.go`, changed-WAL and malformed-sidecar tests. Actual database/WAL bundle acceptance passed. |
| Distinct catalog, tag, player, scrobble, and annotation records | Explicit database record kinds retain all SQL fields. The actual bundle's kind counts were checked. Its catalog publishes 8,038 tracks and 6,894 genre entries. |
| Verify timestamp meaning | Upstream submission-time handling inspected and linked in the notes. Original meaning remains submission. Time offers explicit meanings. |
| Reconcile cumulative counts and events | `count_matching.go`, `count_programs.go`, `inference.go`. Synthetic matching-toggle coverage and actual 149/41/85/23 counter check pass. |
| Use local metadata without replacing conflicts | Source catalog snapshots, recording identifiers, complete records, and source genres remain separate. Existing metadata/genre tests and browser membership verification pass. |
| Multi-Scrobbler inputs, components, parents, queues, and results | Complete SQL envelopes and parent references. Both located schema variants were imported repeatedly. |
| Source plus two destinations is one play | Public fixture asserts one play with six supporting records. Actual inspection confirms multiple destination copies under one play key. |
| Retries and delivery records add no plays | Non-play evidence attaches after attribution. Queue/event/log records remain evidence. Cache duplicate entries match one source event. |
| Version-aware older cache/log reading | Observed Flatted/Typeson and artist variants handled. Unknown serialized types remain context. Actual cache and all 95 located logs passed import/repeat checks. |
| Distinct routing metadata and extra fields | Common typed fields expose service/app/destination/device identifiers. Raw fields survive. Missing routes and parent problems remain unknown/unresolved. |
| Empty backups and unused sources | Empty database records and component records have no playback interpretation. The older schema snapshot has six metadata records and no source plays. |
| ListenBrainz, Maloja, Koito exports | Preserved-file readers, matching and unknown-field tests, and existing legacy importer entry points retained. |
| Needed earlier normalized JSONL/SQLite | Original payload interpretation, derived-row retention, original/converted/derived states, and unchanged existing Last.fm parser identities. Actual normalized files and large ledger/Koito archives checked. |
| Repeated snapshots and retries do not inflate | Changed-WAL test, native-event identities, repeat-import checks, actual consecutive application rebuild fingerprints. |
| Every supported path preserves unknown fields | Complete record payloads, full SQL fields, encoded cache/type information, log lines, and immutable original files. Public preservation assertions and actual download hashes pass. |
| Failed work preserves the usable view | Broken-WAL and mixed-archive rollback tests. Unsupported schemas keep their original file bytes. |
| Minimal interface and established taxonomy | Existing Imports action and filter manager retained. Browser checks show six headings, typed conditions, hover help, and usable source selection. |
| Local development | Native Windows backend on port 4110, with a pre-import database backup. No production deployment. |
| Required Go/client checks | Full Go suite, focused final schema checks, client typecheck, and production build passed. |
| Browser and small screens | Imports, filters, source fields, inspection, and track membership checked at desktop, 390px, and 320px. |
| Documentation | `phase-6.md` records source coverage, actual results, defaults, and later-phase boundaries. |
| GitHub mirror | Closing commit pushed to `origin/custom/koito`. The local and remote commit identifiers were compared before completion. |

## Local verification artifacts

Private logs are intentionally untracked:

- `phase6-final-go.log`, `phase6-final-reader-tests.log`, and `phase6-final-schema-tests.log`
- `phase6-final-typecheck.log` and `phase6-build.log`
- `phase6-combined-actual.log` and `phase6-large-readings.log`
- `phase6-bundle-reading.log`, `phase6-real-cache-logs.log`, and `phase6-real-nav-counts.log`
- `phase6-main-acceptance.log` and `phase6-browser.log`
- Browser images named `phase6-*.png`

The public acceptance hooks are `TestLocalPhase6SQLite`, `TestLocalPhase6Readings`, `TestLocalPhase6MultiHistory`, and `TestLocalPhase6NavidromeCounts`. Their environment variables accept private local paths and are skipped by ordinary test runs.

## Explicit boundaries

A standalone source export is preferable to a processed historical view. Readers retain the distinction. Metadata that remains in complete source records is available to filters and inspection, even when the primary-name projection does not reproduce every legacy metadata choice. The existing importers remain available.

Future cache types, incompatible database schemas, unrelated binary files, and unfinished game/concert/live-capture formats are not guessed. Their originals remain retained or receive the supported direct unsupported-input result. The later phases retain their original scope.
