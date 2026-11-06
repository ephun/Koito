# Phase 2: Last.fm and matching

Status: complete, verified locally on Windows. The full requirements
are in the [development plan](development-plan.md#phase-2-lastfm-imports-and-first-matching-controls).

The current interface follows the [Filters layout](filters-interface.md).
One Save action updates rules and the view.
Hover help explains settings.
Record inspection opens from Imports and listening history.
The original preview and version APIs remain available internally.

## Imports

The existing Import file action detects Spotify extended history and Last.fm
content, including supported members of ZIP uploads. Last.fm readers accept:

- Headerless artist/album/title/date CSV, including a UTF-8 BOM.
- Named-column CSV with Artist, Title and Timestamp; Album may be missing.
- Native recenttracks API envelopes, saved page arrays, and track arrays.
- The earlier Last.fm JSON/JSONL conversions, including retained payload_json.

Original files and every extracted record occurrence remain preserved. A repeated
or renamed upload, or an archive member uploaded separately, reuses its records.
Distinct exports retain their own records. Unsupported content receives a direct
error; failed imports retain their original downloads and import-history entry.
All record insertion for one upload succeeds or rolls back together.

Source labels remain available internally for old imports. The upload interface
uses the single local owner and has no account or importer selector.

## Times

Spotify extended-history `ts` reports a UTC end time; `ms_played` reports actual
played milliseconds, not recording duration. This is specified in the English
field descriptions of the ReadMeFirst PDF accompanying the local export.
The default rule estimates a start by subtracting played time. The alternative
Reported end setting retains end semantics and prevents automatic comparison
with Last.fm starts.

Last.fm scrobble timestamps represent starts, as documented by
[track.scrobble](https://www.last.fm/api/show/track.scrobble). Native
[recent-track responses](https://www.last.fm/api/show/user.getRecentTracks)
retain UTC epoch seconds and now-playing status separately. A now-playing
record never becomes a completed play by itself.

CSV dates preserve their original text and precision. A wall time requires an
explicit timezone rule. The application does not choose UTC or the browser's
timezone for an unknown source. Ambiguous and nonexistent daylight-saving times
remain uncounted with a reason. A converted CSV's appended Z does not establish
that the original wall time was UTC; retained original fields take precedence.

Filters provides a default Last.fm timezone and overrides for individual files.
An explicit blank override means unknown; disabling it allows the broader rule
to apply again. Time interpretation is reversible and leaves records unchanged.

## Matching order and settings

1. Read the original fields and apply the selected time interpretation.
2. Apply the existing qualification baseline: Spotify identified music reported
   complete, and Last.fm identified dated music. Keep every uncounted record
   and its reason. Broader qualification controls belong to later phases.
3. Apply saved manual matches and splits to usable records. Preserve decisions
   concerning temporarily unusable records for future rebuilds.
4. If automatic play matching is enabled, link exact duplicates within a source,
   respecting manual splits. Source identity, time precision, original timestamp,
   and music identity are part of this comparison.
   Repeated minute-resolution Last.fm rows within an export remain ambiguous;
   they can represent genuine short repeats. Their presence also prevents
   arbitrary assignment of copies from overlapping exports.
5. Compare remaining candidates using reliable common identifiers or exact music
   fields. Enabled music settings allow Unicode case-insensitive comparison and
   a missing album. Conflicting known albums require a reliable common identity;
   conflicting reliable identifiers prevent an automatic match.
6. Compare compatible start/end meanings and timestamp precision intervals,
   with an explicit tolerance in seconds. The initial tolerance is zero.
7. Accept only mutually unique candidate partners. Evaluate ambiguity before
   joining pairs, so nearby repeated plays cannot merge through a timing chain.
8. Choose the preferred source's displayed fields within each play. Keep unmatched
   qualifying records from either service as plays. Preserve all reported albums
   and other original fields in contributor inspection.
9. Publish the plays, contributing-record links, ambiguity candidates and display
   rows together in one transaction.

Disabling the music rule removes its case and missing-album allowances; exact
original identity remains available to play matching. Disabling play matching
leaves each qualifying record separate except for explicit manual matches.
These are the first supported controls, not the full future filter taxonomy.
Automatic matches do not cross distinct labels of the same service. Shared-listener
attribution and wider metadata cleanup remain later work.

## Preview, apply, and undo

Preview uses the same matching calculation as rebuilding and writes no state.
It reports before/after play counts, changed results, ambiguous and uncounted
record counts, and representative record groups. Record IDs open inspection of
original fields, contributing records, possible partners, times and file downloads.
Manual match, split and Use rules actions also go through preview and apply.

Apply verifies the preview against the active rule version, records, manual
decisions and latest rebuild. A stale preview is rejected. The previous view
remains available until the complete replacement commits. Failures roll back
rule changes, manual decisions and the replacement together.

Rule versions and manual changes are append-only. Restoring rules creates a new
version and preserves manual matches/splits. Undoing a manual change appends the
previous decision for that pair; it cannot erase a newer edit to the same pair.

## Database changes and compatibility

Migration 004 adds saved rule versions, active settings and manual decisions.
Migration 005 gives listens a separate play key, allowing genuine distinct plays
at one track and whole-second time. It retains existing rows with an empty key,
recreates the existing listen indexes and adds contributor/candidate indexes.

Rebuild deletes only rows owned by the preserved-import view. Remaining legacy
rows and aliases survive; overlaps with unowned rows are reported separately.
Original files and records are never deleted by rebuilding.

Listening-history pagination retains play identity. Imported rows link to record
inspection; the old delete action is restricted to legacy rows. Export pagination
includes the play key in its cursor so equal track/time pairs survive page edges.
The existing view export remains a view export; a complete files/records/rules
backup is planned separately.

Automatic rollback of migration 005 is refused because the old key could collapse
distinct plays. To return to an older executable, restore a pre-migration database
backup together with its preserved source files.

## Verification

Focused automated checks cover both CSV forms, native/converted JSON and JSONL,
file preservation and repeats, unknown fields, sparse albums, timezone scopes,
daylight-saving ambiguity, start/end meaning, case handling, ID conflicts,
mutually ambiguous candidates and timing chains, manual decisions across rebuilds,
preview purity, stale previews, atomic failure, restore and migration preservation.

Export acceptance includes 1,005 distinct plays at the same track and timestamp,
crossing the 1,000-row page boundary. It checks the complete exported count,
history play identity and protection from the legacy delete operation.

Native browser checks exercise the single import action, contributor inspection,
split, manual undo, automatic matching disable/restore, repeated imports, invalid
and explicit timezones, ambiguity inspection, and desktop/mobile layouts.

The optional `TestLocalPhase2Sources` test accepts a JSON array of absolute local
paths in `KOITO_TEST_PHASE2_FILES`. It checks original bytes, repeat imports,
record hashes, cross-service matches, exact contributor links after rebuilding,
and rule restoration. UTC is an explicit acceptance-test setting, not a claim
about the timezone of every private CSV. Real data and screenshots stay outside Git.

Real-data acceptance passed with six original/local source files: both Last.fm
CSV layouts, converted JSONL, native saved Last.fm pages, and Spotify extended
history for two overlapping periods. Results with the explicit UTC test setting:

| Check | Result |
|---|---|
| Preserved record occurrences | 477,574 |
| Plays at zero extra tolerance | 189,831 |
| Plays supported by both services | 5,648 |
| Ambiguous records at zero extra tolerance | 3,483 |
| Plays at two seconds extra tolerance, preview only | 189,830 |
| Ambiguous records at two seconds extra tolerance | 7,178 |
| Rebuild and restore | Exact supporting-record links reproduced |
| Original downloads and raw records | Byte checks passed; record hash unchanged |

The increase in ambiguity supports retaining zero extra tolerance as the initial
setting. Timestamp precision intervals still participate at zero tolerance.
These counts describe the six acceptance inputs and test settings, not a final
count of the user's personal lifetime listening.

The complete import/repeat/rebuild/preview/restore acceptance sequence took
5m05s on the local Windows machine. Initial rebuild finished after 29s including
imports; the first timezone apply finished after 1m30s. A per-play legacy-overlap
query initially caused excessive history scans; rebuilding now reads legacy keys
once, and representative-record lookup has an index. Larger migration and further
performance work remain in Phase 9.

The full Go suite, focused Go vet checks, client type checking and production
client build pass. Browser acceptance uses the native production build with a
fresh database. It also checks stale-preview rejection and recovery without
requiring a page reload. Converted-record validation checked all 141,200 local
JSONL records: none relied on replacing an original blank with a wrapper field.
An additional fixture verifies that a derived album cannot fill an original blank.

The wider tolerance result is not necessarily a lower count: additional candidate
partners can make a formerly unique match ambiguous. The view keeps those records
separate until a rule or manual decision establishes their relationship.

## Completion audit

| Requirement | Evidence |
|---|---|
| Both CSV layouts, BOM, native pages/arrays and converted JSON/JSONL | lastfm_test.go plus the six-file local acceptance |
| Original files, full fields, row occurrences and repeat-upload identity | preservation tests, mixed ZIP/member test, original-download HTTP test and local byte checks |
| Time meaning, precision, unknown zone and daylight-saving ambiguity | normalize tests, time-setting matching tests and native timezone browser flow |
| Saved type/scope/settings/enabled state and versions | rules storage, file-scope test, version restore tests and Filters browser controls |
| Preview purity, atomic apply, undo and stale detection | matching_test.go, filters_test.go and stale-preview browser flow |
| Persistent manual matches and splits | rebuild/restore tests and native match/split/undo browser flow |
| Known overlap counts once; genuine repeats survive | known-pair, timing-chain, coarse-multiplicity and real cross-service checks |
| Sparse/conflicting metadata and per-record albums survive | preferred-release, missing/conflicting-album and conflicting-ID tests; contributor inspection |
| Unmatched records survive source preference | preferred-source test with a Last.fm-only play |
| Correct display/export identity at equal timestamps | 1,005-play pagination/export test and history record links |
| Existing data survives migration and failed rebuilds | additive migration, legacy alias/listen, rollback-refusal and failed-publication tests |
| Minimal Imports UI and working Filters on desktop/mobile | production-build browser checks and local screenshots |
| Local development and GitHub mirror | native Windows checks; working branch committed and pushed |
