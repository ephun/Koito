# Phase 4: implementation progress

Status: **in progress**. Updated November 3, 2026.

The [phase plan](development-plan.md#phase-4-music-matching-album-distinctions-and-bulk-cleanup) defines the complete scope.
The [filter design](filters-interface.md#agreed-next-layout) defines the agreed atomic builder.

## Track histories across albums

The first milestone introduces separate shared histories, album appearances, and recording identities.
Existing catalog track IDs remain usable in URLs and listening records.
One shared history gathers appearances with the same title and artist associations.
Recording identities remain separate within that history.
Version labels in titles retain their existing distinctions.

The additive SQLite migration creates these relationships:

| Table | Responsibility |
|---|---|
| `music_tracks` | Album-independent, name-based track histories |
| `track_histories` | Membership of catalog appearances in those histories |
| `recordings` | Supplied recording identifiers or an explicitly unidentified appearance |
| `track_recordings` | Recording identity associated with each appearance |

The existing `tracks` table supplies album appearances for compatibility.
Each listen retains its original appearance link and timestamp.
MusicBrainz recording identifiers can span multiple album appearances.
Spotify identifiers are preserved in recording associations during projection.
Unidentified appearances retain their own recording identity.
Equal names can share a history while their recording identities remain distinct.

Projection maintains the relationships in the same transaction as the replacement view.
Startup establishes histories for existing catalog entries.
Case comparison for history titles follows the saved music matching setting.
Changing that setting can regroup histories while preserving original aliases and source fields.
Artist and album cleanup remain part of the subsequent filter work.

## Display and API

- Track details include album appearances, their play totals, and recording identities.
- Track histories include listens across album appearances.
- Listening responses include the album associated with each play.
- Track pages link their albums and show album names beside listening rows.
- Track charts, track counts, first listens, activity, and interest use shared histories.
- Album counts and histories continue to follow each play's album appearance.
- All-time track totals include supported undated contributions.
- Calendar views continue to use dated listens.
- Search returns one result for each shared history.
- Small screens use responsive graph widths and a fitting activity grid.

## Existing live capture

Recording-ID lookup now includes the submitted album when associating a live listen.
A shared recording identifier can therefore reuse or create the appropriate album appearance.
Conflicting recording identifiers retain separate appearances, including within one album.
An unidentified appearance remains distinguishable from a newly identified recording.
An absent album is displayed as Unknown Album.
Recording identifiers remain available in exports for each album appearance.

Original fields from existing live submissions require the preservation work planned for Phase 8.
Earlier collapsed album associations require original evidence to reconstruct.

## Verification

All Go package tests pass using the native Windows SDK.
The frontend passes type checking and the production build.

New regression coverage checks:

- Shared track histories across singles, compilations, and absent albums.
- Correct album counts and album identifiers in track charts.
- Live submissions sharing a recording identifier across albums.
- Repeated submissions reusing their appropriate album appearance.
- Conflicting recording identifiers within one album.
- Unidentified recordings remaining distinguishable from supplied identifiers.
- Live, acoustic, and remaster title suffixes retaining distinct histories.
- Reversible Unicode case comparison and unchanged original titles.
- Source-record links and repeat rebuilding across imported album appearances.
- Recording identifiers surviving export across album appearances.

Private acceptance used the existing imports, including Spotify, Last.fm, and iTunes.
Both isolated rebuilds retained **191,473 records and 168,424 plays**.
The activated local application retained the same totals after rebuilding.
These are the current view's totals, rather than an asserted personal lifetime count.

Fingerprints verified unchanged records, file information, contributor links, play timestamps, and manual decisions.
All three original files retained their SHA-256 hashes.
Comparison against the pre-migration backup verified every original catalog row remained unchanged.
SQLite foreign-key checks passed.

A real track page showed 521 plays across a named album and Unknown Album.
Their album totals were 519 and 2.
Each displayed play's album was checked against its original representative record.
Browser checks passed at desktop width, 390 pixels, and 320 pixels.
The browser reported no page errors.

The private database backup and browser artifacts remain under `.local`.
They are excluded from Git.

## Remaining Phase 4 work

1. Extend the implemented atomic evaluator across the remaining filter types and source metadata fields.
2. Migrate existing configurations while retaining their behavior and disabled settings.
3. Replace the mixed filter controls with the consistent IF/THEN/ELSE builder and manager.
4. Add reversible music aliases, source-specific naming, edition grouping, and bulk cleanup.
5. Show affected examples for pattern edits and preserve explicit exceptions and undo.
6. Add source and manual genres with artist genre assignment and chart filtering.
7. Add optional source and metadata tags to listening rows.
8. Integrate contextual corrections with native search and audit the remaining legacy editing operations.
9. Verify every Phase 4 acceptance criterion against the completed implementation.

## Atomic engine and first integration

The typed engine now implements nested IF, THEN, ELSE IF, ELSE, and SET blocks.
Logical operators include AND, OR, NOT, NOR, NAND, XOR, and XNOR.
Comparisons, text transformations, regular expressions, exact arithmetic, dates, durations, lists, and list aggregates compose as expressions.
Missing values, explicit null, empty text, false, zero, and unresolved calculations retain their distinctions.
Unknown conditions leave the complete filter unapplied, including earlier actions within that filter.
Dates preserve sub-millisecond precision. Numeric calculations preserve integers larger than JavaScript's exact integer range.

Presentation, Naming filters can now store and execute these programs through the existing save endpoint.
Each filter compiles once before publication. Its actions change derived names after play matching.
Conditions can inspect normalized music fields, source identity, file identity, device, timestamp, and supplied track identifiers.
Original source records and recording identifiers remain preserved.
Disabling a naming filter restores its original names on rebuild.
Invalid changes roll back the saved version and replacement view together.

The naming filter manager and block editor are implemented.
Other filter types still use their previous configurations pending equivalent program migration.
Raw source fields and unresolved explanations are implemented in the later milestone described below.
The display and artwork changes discussed separately remain deferred at the user's request.

Regression tests cover branching, all Boolean truth tables, incomplete data, atomic actions, exact numbers, timestamp precision, and invalid programs.
Source integration tests cover applying naming programs, disabling them, repeat rebuilding, unchanged source payloads, and failed-save rollback.

## Naming manager and block editor

The Filters page now includes Presentation, Naming with a filter manager.
Rows summarize IF conditions and THEN actions beside the filter's enabled state.
The editor supports nested expressions, nested IF blocks, ELSE IF, ELSE, and ordered SET actions.
Filters support names, duplication, removal, and draft discard.
Source and file comparisons offer choices from the actual imported sources and filenames.
The operator and field registry comes from the backend's validation schema.
Number conversion blocks accept exact numeric text for large constants.

Check changes shows actual before-and-after naming examples beside the editor.
Examples compare the saved configuration with the draft for represented plays and count contributions.
Examples become stale when the draft or saved version changes.
Saving publishes the configuration and derived view together through the existing version checks.

Browser acceptance used a separate copy of the existing imported database.
A conditional uppercase naming filter produced the expected examples and saved through the interface.
Disabling it restored original names and a verified track's 521-play history.
Both saves preserved 191,473 records and 168,424 plays.
Desktop, 390-pixel, and 320-pixel checks passed without browser errors.
Additional checks verified source choices and draft discard at 320 pixels.

The local application exposes the editor with the existing configuration unchanged.
Remaining work includes migrating the older filter controls into the same manager and completing the other Phase 4 requirements.

## Direct attribution migration

Direct attribution now uses the same IF/THEN/ELSE and SET editor as naming.
SET Listener offers Me, Someone else, and Unknown.
New attribution filters start disabled. Their blocks remain editable while disabled.
Priority is available behind a collapsed control.
Higher priorities win. Conflicting assignments at equal priority remain unresolved.

Existing direct-attribution settings receive equivalent editing programs.
Every source, record, device, artist, library, track, file, and interval condition is retained.
Previous specificity becomes an explicit priority.
Presence checks preserve the earlier treatment of missing condition fields.
Saved history stays unchanged. Programs are persisted when the edited configuration is saved.
The legacy API representation continues to execute through its original scope behavior.

Migration tests compare original and converted attribution across matching and mismatching records.
They cover disabled settings, absent metadata, date boundaries, priorities, and conflicting assignments.
Backend tests also verify ELSE behavior, disabling a program, and unchanged source records.
The full Go suite and frontend type checking and build pass.

Browser acceptance used the isolated copy of the actual imported history.
A source-based attribution filter changed the displayed play count from 168,424 to 159,322.
Disabling it restored 168,424 plays. All 191,473 records remained stored.
The editor passed 390-pixel and 320-pixel checks without browser errors.

Matching, inference, heuristic attribution, grouping, genres, and metadata controls remain in Phase 4.

## Imported fields in filter programs

Naming and Direct attribution programs can read arbitrary preserved source fields.
The editor's Source fields menu discovers fields from every record in a selected import source.
Loading fields expands the menu. Source conditions remain explicit IF expressions.

Discovery found 106 iTunes fields, including Genre, and 24 distinct Spotify fields in the actual imports.
Some Spotify fields have multiple observed types, yielding 31 field/type entries.
The imported Last.fm records retain their column and value lists.
Named CSV columns also receive readable field entries.
Object keys use JSON Pointer escaping internally. GET and KEYS handle nested objects and lists.
Array indexes remain accessible through GET without adding every array member to the menu.

Values preserve missing fields, nulls, empty strings, zero, false, and exact large numbers separately.
Rebuilding retains only the raw fields referenced by enabled programs.
Naming examples load fields needed by both the current and proposed configurations.
The source field catalog is a derived cache. New records invalidate its cached discovery.
Original files and record payloads stay authoritative.

Record inspection includes collapsed explanations for unresolved program conditions and values.
These checks evaluate enabled programs against the inspected record's own fields.
They are diagnostics rather than a claim that the record represents a published play.

Tests cover pointer escaping, CSV columns, nested objects and lists, exact numbers,
cache refresh, missing-field explanations, reversible naming and attribution, and unchanged original payloads.
The full Go suite, frontend type checking, and production build pass.
The source-field editor passed actual-import checks at desktop, 390 pixels, and 320 pixels.
Actual-import acceptance saved a raw-field naming filter and displayed an unresolved iTunes-field condition on a Spotify record.
Disabling the test filters restored 168,424 plays. All 191,473 source records retained identical fingerprints.
The main configuration remains unchanged.

Genre assignment, genre charts, and optional metadata on listen rows still need implementation.

## Aggregate attribution migration

Aggregate attribution now uses IF, THEN, ELSE IF, ELSE, and SET blocks in the shared manager.
The My share (%) action accepts a whole percentage from zero through one hundred.
Its value can come from arithmetic, imported metadata, or a constant.
New filters start disabled with an iTunes condition and a zero-percent action.
The same editor supports conditions, priorities, duplication, removal, and draft discard.

Existing shares receive equivalent programs with their original conditions and enabled states.
Previous specificity becomes explicit priority. Saved versions remain unchanged until the next save.
Direct attribution retains precedence over aggregate shares.
Unknown conditions or invalid calculated percentages leave attribution unresolved.
Equal-priority conflicts also remain unresolved.

The engine validates numeric target ranges and whole-number requirements.
Literal errors fail compilation. Invalid calculated values produce inspectable issues and discard that filter's actions.
The aggregate percentage is applied to the interpreted cumulative count, rounding down to whole plays.
The Count and Time inference filters retain their separate responsibilities.

Regression tests compare legacy and migrated scopes, disabled settings, priorities, and conflicts.
They also cover branch selection, arithmetic percentages, missing metadata, reversal, rebuilding, direct precedence, and unchanged original records.
The full Go suite and frontend type checking and build pass.

The existing matching and inference controls still need migration to the shared editor.

Actual-import browser acceptance enabled Count and a temporary 50% Aggregate filter on the isolated database copy.
It checked 5,348 resolved shares against their imported counts and verified whole-play rounding.
The test configuration displayed 190,199 plays. Restoring the original configuration restored 168,424 plays.
All 191,473 records retained identical fingerprints. Desktop, 390-pixel, and 320-pixel checks passed without page errors.

## Remaining legacy editing audit

Merge routes and controls were removed earlier. Other catalog edits still write directly to the catalog.
The following paths remain unfinished Phase 4 work.

| Existing operation | Current behavior | Required replacement |
|---|---|---|
| Alias creation, removal, and primary-name selection | Writes alias rows and primary flags directly | Reversible Music matching and Naming corrections |
| Artist, album, and track deletion | Deletes catalog rows. Track deletion also cleans orphaned entries | Remove routine destructive cleanup and retain reversible view decisions |
| MusicBrainz identifier editing | Updates catalog identifiers directly | Preserve original identifiers separately from corrections |
| Artist membership and primary-artist editing | Changes catalog associations directly | Reversible association corrections with original multi-artist data retained |
| Various-artists editing | Changes the album classification directly | Reversible classification corrections |

The active routes are in `engine/routes.go`.
Their database writes remain in `internal/db/sqlite/artist.go`, `album.go`, and `track.go`.
The Edit modal still exposes aliases, identifiers, artist associations, and the various-artists flag.
The media toolbar still exposes deletion. Completing Phase 4 requires replacing these routine editing paths.

## Heuristic attribution and evidence collections

Heuristic attribution now uses the same IF, THEN, ELSE IF, ELSE, and SET editor.
Collections such as Personal plays from Last.fm provide distinct matching plays assigned by Direct filters.
COUNT compares that evidence with a chosen threshold. WHERE adds explicit conditions on collection items.
MAP reads or calculates item values for SUM, MIN, MAX, or other list operations.
GET reads item fields, including time, artist, album, title, and device.
Item values remain local to the collection expression.

Migration retains source, file, artist, device, library, record, and date conditions.
Date-limited evidence becomes an explicit WHERE expression using its original bounds.
Existing minimum matches become COUNT comparisons. Library shares become percentage assignments.
Disabled settings remain disabled, and previous specificity becomes explicit priority.
Saved versions remain unchanged until the edited configuration is saved.

Direct attribution runs before Heuristic attribution and remains authoritative.
Heuristic assignments contribute no personal evidence to other heuristics.
Unknown conditions and unavailable sources remain unresolved.
Percentages apply to cumulative counts. Applying percentages to individual observed plays produces an inspectable issue.
WHERE requires known Boolean results, including explicit handling of missing item fields.
Nested calculations share an evaluation limit and report exhaustion as an unresolved result.

A derived evidence cache supports record inspection without rescanning the full imported history.
It preserves distinct play keys, original source record links, source identities, and exact interpreted timestamps.
Its publication shares the configuration and view transaction.
Inspection restricts cached evidence to the original owner.
The cache can be rebuilt entirely from preserved records and filters.

Regression tests compare legacy and migrated attribution across date windows, thresholds, scopes, and disabled configurations.
They verify direct evidence requirements, library shares, missing-source explanations, owner isolation, and publication rollback.
List tests cover nested item scopes, exact arithmetic, missing fields, and calculation limits.
The full Go suite and frontend type checking and build pass.

Actual-import acceptance assigned Last.fm history through a temporary Direct filter and evaluated Spotify records with personal matching evidence.
It identified 4,182 Heuristic plays and displayed 165,827 plays under that test configuration.
Eight record inspections completed in at most 0.016 seconds each without unresolved conditions.
All 191,473 source records retained identical fingerprints.
The builder passed desktop, 390-pixel, and 320-pixel checks without page errors.
Restoring the original configuration returned 168,424 plays with unchanged source records.
Acceptance found a missing cache-removal index. The corrected index restored the baseline in 84.3 seconds.
Publication also clears child links in bulk before removing cached evidence.

Unedited conversions preserve legacy conflict decisions through migration fingerprints.
Editing their blocks compares current assignments independently from hidden legacy settings.
Restoring the original blocks restores their original conflict behavior.
Table summaries use source names and available value labels, with complete nested THEN, ELSE IF, and ELSE actions.
The updated main application rebuilt in 72.406 seconds and retained 168,424 plays and 191,473 records.
Original-file checks verified all three SHA-256 hashes, and contributor, timestamp, and decision fingerprints remained unchanged.

Remaining work includes matching and inference migration, reversible music aliases, edition grouping, genres, optional tags, and contextual catalog corrections.
