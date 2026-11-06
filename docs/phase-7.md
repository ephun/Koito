# Phase 7: concerts and gameplay

Status: complete. Development and verification remain local on Windows.

## Requirements

- Preserve original Setlist.fm JSON responses and supplied attendance evidence.
- Retain concert identities, revisions, dates, venues, performers, sets, song order, covers, and encores.
- Reconcile revisions without adding another concert. Retain genuinely repeated songs.
- Recover retained legacy concert records and references without treating generated timestamps as observations.
- Preserve date precision and distinguish date-only performances from estimated song times.
- Import YARG SQLite games, player scores, players, and supplied metadata as distinct records.
- Verify YARG timestamp meaning and timezone handling against upstream code and actual files.
- Match repeated snapshots and corresponding YARGfm scrobbles without multiplying games or listens.
- Expose replay, bot, speed, completion, and player conditions through the established filter types.
- Verify Clone Hero binary versions before interpreting scores or song caches. Preserve unsupported originals and return direct errors.
- Make gameplay and concert inclusion configurable through Filters.
- Verify original bytes, complete records, repeat imports, changed snapshots, rebuilds, failure handling, and real-data browser behavior.
- Run applicable Go and client checks, document limits, and mirror the completed phase to GitHub.

## Located evidence

The release YARG database has 568 game records, 996 player scores, and 22 players. The nightly database has the same three table families and no rows. Its older schema omits `HasBots`.

The gameplay export also contains YARG song caches, player profiles, and Clone Hero score, profile, and song-cache binaries. These are separate forms of evidence. A score does not establish a dated listening event by itself.

The earlier survey found 1,132 concert-derived listens in the Koito backup and no standalone original Setlist.fm response. Recovery and completeness are being checked against the actual retained records.

## Verified format references

- [YARG game record](https://github.com/YARC-Official/YARG/blob/f5bfbe6996b4af35722d4204a93c6cc5c8f52a45/Assets/Script/Scores/GameRecord.cs)
- [YARG score recording](https://github.com/YARC-Official/YARG/blob/f5bfbe6996b4af35722d4204a93c6cc5c8f52a45/Assets/Script/Gameplay/GameManager.cs)
- [YARG score schema](https://github.com/YARC-Official/YARG/blob/f5bfbe6996b4af35722d4204a93c6cc5c8f52a45/Assets/Script/Scores/PlayerScoreRecord.cs)
- [Setlist.fm setlist and revision fields](https://api.setlist.fm/docs/1.0/json_Setlist.html)
- [Setlist.fm attendance responses](https://api.setlist.fm/docs/1.0/resource__1.0_user__userId__attended.html)
- [YARG release cache reader](https://github.com/YARC-Official/YARG.Core/blob/3beb94e526558134145bcd3e409428f759001a40/YARG.Core/Song/Cache/CacheHandler.cs)
- [YARG nightly cache reader](https://github.com/YARC-Official/YARG.Core/blob/49ad2e83fdb86010d83c0abbaf620dea5e5f3eff/YARG.Core/Song/Cache/CacheHandler.cs)
- [Clone Hero cache structure](https://github.com/sonicv6/CHParser/blob/6e987e2036fafc18da632db4a3c0c6cd155aed59/CHParser/Models/SongCache.cs)
- [Clone Hero cache entries](https://github.com/sonicv6/CHParser/blob/6e987e2036fafc18da632db4a3c0c6cd155aed59/CHParser/Models/SongEntry.cs)
- [Clone Hero score structure](https://github.com/coolcarp/clonehero-score-exporter/blob/ce0c4a287dcc419741bb1d44752dd7aa54e80310/main.py)

The inspected YARG code records scores after reaching the song end and stores `DateTime.Now`. The database retains ticks without a timezone. Interpret that value as the local time when the game result was recorded. An explicit Time filter supplies the timezone. Result-recording time remains distinct from audio start, audio end, and scrobble submission.

The user's [YARGfm implementation](https://github.com/ephun/YARGfm/blob/6ea644e8d8bd27d674b1c5070697919423516770/yargfm.py) converts that same game date to a whole-second local timestamp before submission. All 29 retained YARGfm observations fall within the corresponding native result second. Subtracting the chart duration would prevent this correspondence. The optional estimated-start filter is a separate interpretation.

## Imported records and original files

| Input | Preserved interpretation |
| --- | --- |
| YARG SQLite | Games, player scores, players, schema, and complete additional columns. Game records include their related score and player context. |
| YARG song caches | Verified versions `25031401` and `26042800`. INI/SNG entries include chart checksums, metadata, duration, available instruments, credits, links, settings, and original entry bytes. Other framed groups remain binary context. |
| Clone Hero scores | Verified versions `20180220` and `20211009`. Separate song summaries and instrument/difficulty scores retain checksums, accuracy components, flags, counters, and entry bytes. |
| Clone Hero song cache | Verified version `20221120`. Catalog entries retain names, paths, categories, checksums, durations, and original entry bytes. |
| Setlist.fm JSON | Individual responses, arrays, and paginated responses. Page metadata, concert headers, sets, song occurrences, cover credits, and explicit attendance remain linked. |
| Legacy concert history | Original observations and payloads from the earlier Koito backup. Explicit dates and retained Setlist.fm references are recovered. Generated legacy clocks remain source information. |
| Legacy YARGfm history | Complete retained observations, with YARGfm distinguished from the native YARG database and its forwarding protocol. |

Repeated files, renamed files, archive members uploaded separately, and identical score backups reuse records. Repeated rows at separate positions remain separate records. Changed database snapshots add new evidence and use original game identifiers to reconcile unchanged games.

Archive members with unsupported formats remain downloadable. Standalone unsupported files return a direct error. Clone Hero profile binaries, settings, and YARG profile/binding files are retained originals, not claimed as decoded score formats. Unknown binary versions are rejected. Truncated supported binaries fail without publishing partial records.

Clone Hero's three-byte score counter has unverified semantics. It remains `counter_bytes` with `counter_meaning: unverified`. These score summaries do not establish dated plays or cumulative listening counts.

The private recovery exports retain every original observation column, the exact nested payload, the source table, and the original database's file hash. The complete original Koito backup is also saved in the application's file store. Recovery exports are derivatives, not substitutes for that backup.

## Filters and view behavior

This phase uses the established types and subtypes:

- **Matching / Music:** Existing comparison-name programs reconcile metadata differences, including capitalization. Source names and albums remain available.
- **Matching / Play:** Original game identities reconcile snapshots. A Gameplay scrobbles starting filter allows overlapping game/scrobble timestamp ranges after music matching. Competing events, saved splits, and conflicting known metadata retain ambiguity.
- **Inference / Time:** Timezone, timestamp meaning, precision, and optional estimated times. YARG result time and date-only concert data retain their distinct meanings. Date calculations run after resolving a missing timezone.
- **Inference / Context:** Assign Gameplay, Concert, or Unspecified using atomic conditions. Named Multi-Scrobbler inputs remain distinct from their submission protocols.
- **Attribution / Direct:** Player identifiers/names and explicit attendance information can assign listening. A multiplayer game produces one play, with every score retained separately. Unassigned game and concert evidence stays unknown.
- **Qualification / Completion:** Replay flags, bot presence, speed, scores, and other retained metadata are available to conditions. The starting game filter requires a positive speed, score evidence, and no reported replay playback.
- **Inclusion / Scope:** Gameplay and concerts can be included or excluded independently. Exclusion changes the view while preserving records, play relationships, and context.

Catalog entries join games by service and chart checksum. Conflicting album/duration candidates remain available, without selecting an unsupported singular value. Record inspection uses the same association as rebuilding. Native game context takes precedence over a sparse matched scrobble's context. A later reading of the same native game supplies updated score context, while earlier readings remain contributors.

Calendar-only concert dates reach listen history and the first-listen summaries on artist, album, and track pages. They render as calendar dates in every browser timezone. They do not render a fabricated song clock. Explicitly estimated times carry an estimated-time marker and hover help. Changing Inclusion also updates those summaries.

## Real-data checks completed during development

- Release YARG: 568 games, 996 scores, 22 players, and one schema record. Explicit attribution to the identified player contributes 541 games.
- Empty nightly YARG: one schema record, without inventing games or missing flags.
- YARG caches: 4,520 release records and 49 nightly records. Repeat imports reuse them. Truncated versions fail.
- Clone Hero: 179 old score records, 227 newer score records, and 481 cache records. The identical score backup reuses all 179 records. These files add no dated plays.
- Legacy concerts: all 1,132 observations survive. Explicit retained dates support 1,059 date-only plays. Another 73 lack a recoverable date, including 57 with retained Setlist.fm identifiers. Completeness of the user's concert history remains unproven.
- Combined acceptance: all 29 retained YARGfm observations and 26 corresponding Last.fm rows support native games. The selected player still contributes 541 gameplay plays. This passes both case-insensitive Music matching and case-sensitive matching with explicit filters for two metadata capitalization differences. Original names remain unchanged.
- The actual archive contains 8,384 record occurrences across supported members. Its duplicate score backup accounts for 179 reuses, leaving 8,205 new records.
- Original archive and all 22 member downloads match their supplied checksums. The complete 2,881,953,792-byte Koito backup download also matches its original checksum.

Unit checks cover corrected revisions, repeated encores, separate concerts on one day, unknown revision order, attendance, cover credits, invalid dates/ticks, multiplayer relationships, changed snapshots, explicit attribution, context changes, catalog conflicts, estimated times, binary bounds, unsupported archive members, and reversible inclusion. Anonymous fixtures exercise all three supported song-cache versions in the ordinary suite, alongside opt-in full private exports.

An undated legacy concert can receive an explicit estimated time through Time. That estimate does not change its retained generated clock. Missing legacy set positions and tape flags remain unknown. Invalid Context values appear as filter issues.

Record inspection formats original JSON tokens directly. Large tick values, identifiers, and numeric exponents retain their original digits instead of passing through JavaScript number serialization. Browser verification checks an actual imported YARG timestamp against the preserved payload.

Full-history verification exposed session expiry updates invalidating SQLite read snapshots during authenticated browsing. Current sessions now renew at most daily. A WAL regression test verifies that an ordinary session refresh leaves a concurrent read transaction writable. Authenticated configuration, Filters, actual game inspection, and history requests also answered during the real rebuild without changing session expiry rows.

The full-history publication retained all 1,104,834 raw records and preserved the exact earlier 1,096,629-record prefix. Two explicit Music filters address capitalization differences while keeping the library's overall case-sensitive matching setting.

The verified main view contains 265,129 source plays, including 1,059 concert plays and 565 gameplay plays. All 29 retained YARGfm observations support native games and retain their native score context. Every visible gameplay group has a native game. The main total includes previously captured listening. The selected-player filter identifies 541 native games, while 24 other-profile games have existing personal scrobble evidence and explicit Direct attribution. Three native games remain unassigned. Parent-only listening stays outside the view.

The corrected filter publication took 864 seconds. A complete repeat rebuild took 1,870 seconds, including verification. Exact hashes remained identical for all 1,104,834 raw records, 476,602 supporting-record links, 1,627 performance contexts, 265,129 source plays, 2,317 snapshots, and 166,737 snapshot items. Foreign-key checks passed. These are measured local results, not performance targets.

SQLite integration checks verify calendar precision through history and all three music-page summaries, exclusion/reversal, and an earlier observed play replacing a calendar-only first listen. Full-data publication and deterministic rebuilding are verified. The final binary restart preserved all six verified table hashes. Every actual game group contains one native game record, so selecting updated context for later snapshots does not change this dataset.

Final main-app browser checks passed for Filters, Imports, exact raw game timestamps, recovered concert dates, and artist, album, and track pages. Desktop, 390px, and 320px layouts fit without page errors. Rankings sit beside titles. The type labels are larger, and listening statistics form one sentence at one size. Music title sizes now override the global heading style correctly.

The complete Go suite passed with the actual-data acceptance tests enabled. Client typecheck and production build passed. JSON formatting checks also cover large integers, numeric exponents, escaped strings, Unicode, and empty containers. The implementation and this audit are included in the closing commit on `custom/koito`.

## Closing requirements audit

| Requirement | Evidence | State |
| --- | --- | --- |
| Original files and complete records | Setlist byte-equality test, raw extra-column assertions, immutable record triggers, original archive/member/backup download checksums | Verified |
| Idempotent imports and changed snapshots | Renamed/repeated input tests, identical score-backup reuse, actual archive repeats, updated multiplayer snapshot test | Verified |
| Concert identities and revision handling | Setlist revision, encore, separate-concert, unknown-order, attendance, and cover-credit tests | Verified |
| Legacy concert recovery and honest time precision | 1,132 retained records, 1,059 explicit calendar dates, 73 unresolved dates, 57 recovered references, explicit-estimate tests | Verified |
| YARG games, scores, players, timestamps, and metadata | Actual 568/996/22 source rows, upstream result-time code, timezone/tick tests, latest native context, catalog conflict checks | Verified |
| Native/YARGfm/Last.fm reconciliation | Both case-sensitive and case-insensitive combined real-data tests retain 541 attributed games and all 29/26 corresponding records. Full history links all 29 YARGfm observations to native games | Verified |
| Verified Clone Hero and YARG binary readers | Documented version gates, anonymous INI/SNG/cache fixtures, opaque framed-group preservation, actual full-file imports, truncation/bounds tests | Verified |
| Existing filter taxonomy and reversible view changes | Context, Direct, Time, Completion, Music/Play matching, and Scope programs, template compilation, reversible inclusion tests | Verified |
| Calendar dates and concise interface | Actual-data desktop/390px/320px browser checks, UTC/Denver/Auckland comparison, final main-app headers and exact tick inspection | Verified |
| Full-history integrity and deterministic rebuilding | Raw-record prefix, foreign keys, contributor/context/snapshot hashes, repeated full rebuild | Verified |
| Applicable checks | Full Go suite, client typecheck/build, focused regression checks, staged-content review | Verified |
| GitHub mirror | Closing commit on `custom/koito`, with remote HEAD checked against local HEAD after pushing | Checked at publication |
