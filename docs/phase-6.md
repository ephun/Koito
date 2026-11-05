# Phase 6: Navidrome, Multi-Scrobbler, and historical exports

Status: complete. The closing commit is mirrored on `custom/koito`.

This phase brings local playback databases, forwarding history, and the remaining history exports through Imports. Original files and complete records remain separate from the view. No new filter categories were added.

## Supported inputs

| Input | Interpretation |
| --- | --- |
| Navidrome SQLite | Catalog tracks, albums, artists, tags, tag links, players, scrobbles, and annotations remain distinct records. Other logical rows remain context. |
| SQLite with WAL | Upload a ZIP containing the database and its matching `-wal`. Member order does not matter. The `-shm` file is retained but rebuilt for reading. |
| Multi-Scrobbler SQLite | Source plays, components, inputs, destination copies, events, queues, and historical rows retain their supplied fields and references. Both located schema variants are supported. |
| Older Multi-Scrobbler cache | Read the located Flatted/Typeson representation, including string and object artist layouts, dates, undefined values, progress values, and shared references. Preserve encoded type information. |
| Multi-Scrobbler logs | Retain complete lines and structured fields. Declared play identifiers can link lines to existing plays. Human-readable song descriptions do not become additional plays. |
| ListenBrainz | Individual listens, JSON arrays, submission payloads, paginated listen responses, and JSONL, including JSONL inside ZIP exports. |
| Maloja | The supported `scrobbles` export shape, including its artist-list convention. |
| Koito JSON | Version 1 exports. All aliases, artist lists, client information, identifiers, durations, and extra fields remain in records. |
| Koito SQLite | Native listens can be read with their track, album, and artist context. A backup containing an evidence ledger retains its observations separately from its materialized listens. |
| Earlier normalized files | The located `raw_play_events` SQLite tables and normalized JSONL forms. Existing Last.fm reader identities remain unchanged. |
| Earlier listening ledger | Preserve observation payloads, cumulative evidence, metadata evidence, source references, and existing event links. Recover retained original payloads where available. |
| Empty backups | Recognize the located empty database backups as metadata. Configured components and empty files establish no captured playback. |

Readers check the required columns before interpreting a recognized database family. Extra columns survive. Unsupported schemas produce a direct error while retaining the original file. Unsupported cache type tags remain preserved context rather than being interpreted as plays.

## Matching, Count, and Time

Multi-Scrobbler destination copies, retries, inputs, queue changes, and submission results support their source play. They are attached after attribution so they do not become additional listeners or independent plays. Missing and cyclic parents remain explicit unresolved references.

Native event identifiers link repeated snapshots. Matching also supports an unambiguous event reported by three or more services. Competing repeats, incompatible pairs, and saved splits remain separate.

Navidrome stores submission timestamps. Those values retain their meaning and supplied precision. Time can explicitly assign another meaning before Play matching compares them with another service. Unknown timestamp meanings remain unknown.

Navidrome annotations enter the existing Count engine. Album and artist summaries do not become additional track counts. The Count starting programs subtract `library.matched_repeats`, labelled **Matched scrobble duplicates**, from the selected reported total. This value comes from Play matching and the selected snapshot's scrobble records. It is zero for iTunes. Custom programs can explicitly choose their own calculation. A counter smaller than its retained scrobble history leaves the overlap unresolved.

The real Navidrome snapshot demonstrates why this matters:

| Evidence | Quantity |
| --- | ---: |
| Scrobble rows | 126 |
| Plays after Matching | 85 |
| Matched duplicate scrobble rows | 41 |
| Reported track-counter increments | 149 |
| Interpreted cumulative total with the starting Count program | 108 |
| Observed plays covered by that total | 85 |
| Remaining undated plays when Count is enabled | 23 |

The original counter remains 149. Disabling Play matching changes the duplicate adjustment with it. Count remains configurable and is disabled in the current local view.

## Metadata and provenance

The Navidrome catalog contributes library membership and source genres independently of playback. Both older genre strings and structured genre entries are supported. Supplied labels and their source fields remain available for Naming and genre filters. Conflicting metadata stays in its own source record.

Filters can distinguish original service, forwarding app, destination, device, source event identifier, original format, played milliseconds, track duration, and native library/track identifiers when supplied. A component's input protocol does not establish an otherwise unknown original service.

Retained Spotify and Last.fm payloads use their existing interpretation paths, including Time and Completion filters. Previously expanded iTunes rows and materialized historical views remain derived. They do not silently acquire observed play dates. Native Koito timestamps with an unestablished meaning remain unknown.

Count's Record fields loader now offers both iTunes and Navidrome. The existing filter manager, IF/THEN blocks, operators, and six categories remain in use. Source labels are shared across Imports, inspection, and filter editors.

The legacy importer entry points remain available. Their metadata handling is retained while these exports also enter the preserved-record path. Full source structures, including secondary credits and image references, remain inspectable even where the current track projection uses a primary name.

## Storage and failure handling

The file limit accommodates the located 2.7 GB backup. SQLite is identified before loading text into memory. Database rows stream through a temporary record file.

SQLite opens an isolated copy of the uploaded bytes. Supplied WAL headers, page sizes, and frame checksums are checked before reading. Missing databases, conflicting sidecar names, damaged frames, incompatible schemas, and failed extraction preserve the previous published view and uploaded originals. Streamed extraction also checks its expected record count before publication. Different WAL contents identify different readings of the same database bytes. Repeated event identities keep those readings from multiplying plays.

Original database, WAL, archive, cache, and log bytes remain downloadable. Table definitions and complete logical row fields are retained. SQLite search-index storage remains in the original database rather than being expanded into music records. Original files and the application database must both be backed up.

## Verification

- Focused tests cover forwarding copies, retries, three-service matching, missing/cyclic parents, schema errors, empty backups, cache type handling, changed WAL snapshots, damaged/missing/conflicting sidecars, counter overlap, separate Navidrome libraries, and structured genres.
- Full Go suite passes. Client typecheck and production build pass.
- Eight real inputs preserved 712,007 records and passed repeat-import checks. They include both located Multi-Scrobbler schemas and the normalized SQLite/JSONL layouts.
- The large archive reader checked 4,551,009 logical records from the Koito backup and 1,421,572 from the older ledger. Stored original checksums remained unchanged.
- The real cache and 95 logs preserve 116,206 records. The cache recovers 17 distinct plays from 21 entries. Logs contribute supporting evidence.
- The actual Navidrome database plus its supplied WAL passed bundle reading. Its catalog has 8,038 tracks. The published view contains 6,894 Navidrome genre entries.
- The local application imported the real playback/forwarding bundle. It added 173,567 records, bringing the total to 543,181. Reuploading reused all 173,574 record occurrences in the bundle, including identical empty members.
- Two consecutive application rebuilds produced 177,958 plays with identical checked records, snapshots, play identities, contributors, music states, and genres. The existing 369,614 source records retained their fingerprint. Foreign key checks passed.
- Original archive, database, WAL, and cache downloads match input checksums. Actual forwarding inspection shows multiple destination copies supporting one play.
- Browser verification covers Imports, source-field selection, the Count expression, record inspection, catalog membership, and desktop/390px/320px layouts. The temporary browser draft was not saved.

Private acceptance logs, browser captures, databases, and source files remain in ignored local storage. Reproducible public fixtures and opt-in private-data acceptance tests are in `internal/sources`.

## Boundaries for later phases

Game/score files, concert/setlist interpretation, Clone Hero binary formats, and live capture remain in their planned phases. Earlier processed estimates remain distinguishable from observations. This phase does not deploy to the server or claim complete capture for configured but unused sources.

## Primary references

- [Navidrome scrobble storage](https://github.com/navidrome/navidrome/blob/master/persistence/scrobble_repository.go)
- [Navidrome play-count and timestamp handling](https://github.com/navidrome/navidrome/blob/master/core/scrobbler/play_tracker.go)
- [Multi-Scrobbler cache serialization](https://github.com/FoxxMD/multi-scrobbler/blob/master/src/backend/common/Cache.ts)
- [SQLite WAL format and checksums](https://www.sqlite.org/fileformat2.html#wal_format)
