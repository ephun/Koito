# Phase 4: implementation progress

Status: **in progress**. Updated November 4, 2026.

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

1. Add optional source and metadata tags to listening rows.
2. Integrate contextual corrections with native search and replace the remaining destructive cleanup operations.
3. Extend previews, exceptions, and undo checks to those remaining actions.
4. Verify every Phase 4 acceptance criterion against the completed implementation.

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

## Count inference programs

Count inference now uses the shared filter manager and conditional editor.
Its programs explicitly select supporting iTunes library export records and assign a cumulative count.
The selected record supplies names and date evidence independently from the count expression.
Counts without a selected date record remain undated.

Starting filters preserve largest-count, latest-count, and counter-reset interpretations.
They remain editable programs after creation.
The largest-count expression uses MAX over the selected reported counts.
Latest and reset fields compute their values from the currently selected supporting records.
Earlier SET actions update those inputs once, without repeating the program.
Reset totals explicitly remain undated because their combined date interval is uncertain.

WHERE and MAP bind readable record fields within each collection expression.
Record fields can load arbitrary metadata from the actual iTunes imports.
Missing fields, empty values, zero counts, and invalid calculations retain distinct outcomes.
File conditions offer the actual iTunes filenames.
The editor keeps Count and Time in separate cards under Inference.
Purpose and field help use question-mark tooltips.

Migration preserves scopes, priorities, disabled settings, selected records, amounts, and timing behavior.
Unedited migrations retain the earlier unidentified-library checks and conflict decisions.
Unknown or invalid count outputs remain unresolved and appear in record inspection.
Original source records and saved configurations remain unchanged.

Regression coverage compares all three interpretations across valid, conflicting, missing, undated, and unidentified records.
It also checks file and record conditions, raw-metadata selection, exact arithmetic, reversal, and repeat rebuilding.
Local item paths remain confined to WHERE and MAP expressions.
The full Go suite and frontend type checking and production build pass.

Actual-import acceptance selected Rock records through the editor and applied a temporary 50% Aggregate filter.
It checked genre selection on 1,777 entries and whole-play rounding for 741 shares.
The test configuration displayed 170,818 plays. All 191,473 source records retained identical fingerprints.
Desktop, 390-pixel, and 320-pixel checks passed without page errors.


Restoring the original configuration returned 168,424 plays in 60.94 seconds.
The updated main application retains the baseline totals and all three original-file hashes.

Time inference, matching, music aliases, edition grouping, genres, optional tags, and contextual corrections remain outstanding.

## Time inference programs

Time inference now uses the shared conditional editor for Last.fm timezones, Spotify timestamp interpretation, and iTunes play dates.
Timezone values use a dropdown. The starting Last.fm filter selects UTC.
Existing unknown timezone settings retain their original behavior during migration.
Spotify time interpretation moves into its own Time filter with the saved end/start choice retained.
Disabling or removing it uses reported endpoints without reactivating the hidden legacy setting.

Time filters support explicit conditions, priorities, raw metadata, and duration adjustments.
Conditions read pre-adjustment timestamps once. Duration arithmetic retains nanosecond precision.
Invalid timezones and excessive durations leave the filter unresolved and appear in record inspection.
Library timing remains dependent on Count and preserves separate last-played and estimated-date actions.
Original timestamps, records, and saved configurations remain preserved.

Regression tests compare migrated behavior across every supported Last.fm layout, unknown and explicit timezones,
file overrides, Spotify starts/ends, disabled settings, and iTunes date combinations.
They also verify raw conditions, atomic invalid actions, conflicts, and removal of migrated Spotify filters.
The full Go suite, frontend type checking, and production build pass.

Actual-import acceptance checked 9,102 Spotify representative plays against their original reported endpoints.
Restoring estimated starts reproduced the full play fingerprint.
Restoring the original configuration retained 168,424 plays and 191,473 records.
The timezone dropdown and editor passed desktop, 390-pixel, and 320-pixel checks without page errors.

Matching, music aliases, edition grouping, genres, optional tags, and contextual corrections remain outstanding.

## Play matching programs

Play matching now uses the shared conditional editor with a Same play Boolean assignment.
Each comparison has stable Left and Right records ordered by record identifier.
Expressions can read both records' music fields, sources, timestamps, devices, files, and arbitrary raw fields.
Timestamp-range overlap, range gaps, comparable time meanings, and exact source duplicates are explicit inputs.

Existing time tolerances become duration comparisons in the migrated program.
Exact copies retain their previous handling, including ambiguous repeated minute-resolution Last.fm records.
Multiple Play matching filters support priority, explicit exclusions, equal-priority conflicts, and missing-field explanations.
An unresolved condition discards the complete filter's assignments.
Manual match and split decisions retain their independent precedence.
Music compatibility, same-owner requirements, and candidate ambiguity remain separate checks.

Candidate indexing derives safe time bounds from recognized expressions.
Unbounded or unrecognized expressions compare the entire matching music block.
The optimization does not impose an implicit one-hour limit on edited filters.
The shared editor offers overlap and 30-second starting filters.
Existing source-preference settings remain stored during migration.

Record inspection can compare a selected second record using the active matching filters.
It shows Music matching and Play matching results separately, with unresolved explanations.
The comparison is read-only and uses the existing record inspection flow.

Regression tests cover migration, disabled settings, duplicates, ambiguity, duration limits, distant candidates,
stable record sides, raw fields, explicit exclusions, priorities, conflicts, missing fields, reversal, and repeat rebuilding.
The full Go suite and frontend type checking and production build pass.

Music matching migration, music aliases, edition grouping, genres, optional tags, and contextual corrections remain outstanding.

Actual-import browser acceptance added an exception for one existing Spotify/Last.fm pair.
The exception changed 168,424 plays to 168,425 and returned FALSE in record comparison.
Restoring the original configuration restored 168,424 plays and the complete play fingerprint.
All 191,473 source records retained identical fingerprints.
Desktop, 390-pixel, and 320-pixel checks passed without page errors.

## Music comparison names and unified editor

Music matching now uses the shared conditional editor.
Filters assign artist, album, and track names used for comparison, plus explicit handling of an empty album.
Original names, comparison names, and displayed names remain separate.
Source, file, artist, device, record, and arbitrary raw metadata can select corrections.
Text operations support case normalization, aliases, regex cleanup, whitespace, and punctuation changes.
Higher-priority corrections run after lower-priority corrections. Equal priorities retain their saved order.

Comparison names feed play matching, personal evidence matching, count-overlap checks, and shared track histories.
Conflicting reliable recording identifiers continue to retain separate recording identities.
Album appearances and their original reported associations remain independently addressable.
Comparison changes do not rename catalog aliases.

When source-specific comparisons differ for equal displayed names, derived album appearances retain separate history links.
Their artist credits, recording links, album, and duration are copied from the existing appearance.
Original catalog rows remain unchanged. Undo reuses the original appearance identifiers.
The variant mapping and history overrides are derived data.
Native catalog histories also evaluate applicable comparison programs while retaining multiple artist credits.

Music matching shows actual before-and-after comparison examples through Check changes.
Examples load the prior configuration's record interpretation separately from the proposed configuration.
Unknown conditions and invalid empty comparison names discard the complete filter and appear in record inspection.
Migration retains disabled settings and the earlier case/missing-album behavior.

All existing filter subtypes now use the same manager and IF/THEN/ELSE editor.
The old scoped controls, fixed matching switches, and their unused client component have been removed.
The configuration can explicitly contain zero filters, remain empty after rebuilding, and restore a previous version.
Legacy configurations remain readable and execute their original paths until converted and saved.

Tests cover every case/missing-album/enabled combination, comparison aliases, source-specific histories,
unchanged display names, recording distinctions, multiple artist credits, preview examples, incomplete metadata,
empty configurations, repeat rebuilding, and restoration of original appearance identifiers.
The full Go suite, frontend type checking, and production build pass.

Actual-import acceptance previewed twelve comparison changes and enabled case-insensitive matching.
The temporary configuration displayed 167,717 plays, compared with 168,424 before the change.
All 191,473 records and the existing 24,450 catalog titles remained unchanged.
Restoring the original configuration returned 168,424 plays and the original play fingerprint.
The preview and editor passed desktop, 390-pixel, and 320-pixel checks without page errors.
Converting the saved configuration alone preserved play, record, and contributor fingerprints across all 168,424 plays.
Restoring its original representation reproduced those fingerprints again.
The main application rebuilt in 61.375 seconds and retained all three original-file hashes.

Genres, optional tags, and contextual corrections remain outstanding.

## Reversible edition grouping

Presentation, Grouping now uses the shared IF/THEN/ELSE editor.
SET Album group chooses a reporting name using original names, displayed names, supplied fields, or an expression.
The starting filter recognizes selected deluxe, expanded, and remastered suffixes and starts disabled.
Empty text creates an explicit exception. Later filters can override earlier assignments.
Naming and Grouping editors support moving a filter up or down within their subtype.
Naming resolves before Grouping. Original release titles and identifiers remain independent.

Groups retain their complete artist-credit identities and stable identifiers across disabling and re-enabling.
Each play retains its physical release association.
Separate grouping links for plays and cumulative contributions support source, file, record, library, and metadata conditions.
Two contributions on the same appearance can therefore remain in different reporting groups.
Native release defaults apply to available catalog metadata and newly captured releases.
An incoming edition can join a previously captured base release immediately.
Complete live-record fields still depend on the preservation work in Phase 8.

Album charts, album counts, and new-album counts use the reporting groups.
Grouped pages show individual editions with their group-specific contributions.
Their track charts, listening history, interest, and activity follow the same membership.
Listening rows link the album associated with each play.
Original edition pages and native search remain available.
Calendar ranges exclude undated amounts. All-time reporting includes those supported contributions separately.

Regression tests cover native and imported grouping, source and library exceptions, complete artist credits,
missing-field explanations, atomic failure, empty exceptions, previews, disabling, repeat rebuilding, and stable group identifiers.
They also exercise incoming editions, existing base releases, charts, date bounds, histories, activity, interest, and undated counts.
The full Go suite, frontend type checking, and production build pass.

Actual-import browser acceptance checked twelve grouping examples and published the editable suffix filter on an isolated database copy.
The Family Tree: The Roots reported 1,743 plays across two editions with listening contributions.
A third known edition remained listed with zero contributions.
All 191,473 records and 168,424 plays remained present.
Record, play, catalog-title, release, and contributor fingerprints stayed unchanged.
Desktop, 390-pixel, and 320-pixel checks passed without page errors.
The original configuration was restored after acceptance.

The activated local application rebuilt in 60.188 seconds and retained the same record and play totals.
All three original files retained their SHA-256 hashes.

The measured preview took approximately 21 seconds and publication took approximately 73 seconds on the imported history.
Performance targets and broader measurements remain in Phase 9.
Phase 4 remains in progress for genres, optional listening tags, and contextual corrections replacing destructive edits.

## Genre evidence foundation

The iTunes readers expose original genre labels separately from interpreted listening.
Labels retain their exact text, including commas, whitespace, empty values, and repeated plist entries.
Each label carries its original field location and source record reference.
Unsupported shapes remain available in the preserved payload.
Migration 13 adds a derived `source_genres` table rebuilt in the publication transaction.
Genre evidence remains available when Count inference is disabled.
Record inspection exposes this evidence in a collapsed Source genres section.

The current imports contain 10,629 library-track records with genre text across 1,286 distinct artist strings.
Source labels include nonmusic classifications, such as Audiobook and Voice Memo.
These labels are evidence rather than automatic decisions about qualifying listening.
Regression tests cover exact labels, repeated values, unsupported shapes, count-independent inspection, and idempotent rebuilding.
Source and database tests pass. Artist assignments, conflict resolution, manual overrides, and chart filtering remain unfinished.

## Artist genre assignments and charts

Migration 14 adds derived artist assignments and unresolved-filter explanations.
Publication resolves artist labels in the same transaction as the view.
Original genre evidence remains in `source_genres` and the preserved records.
Exact artist aliases link evidence only when the alias identifies one catalog artist.
Unlinked or ambiguous labels remain available in record inspection.

Naming filters can assign Artist genres using LIST, source label collections, and conditional expressions.
Source genre records retain file, source, record, and field references for WHERE and MAP conditions.
Artist genre filters evaluate separately from record naming filters.
Mixing those actions within one filter fails validation during compilation.
Genre conditions use ordinary IF or collection expressions rather than legacy scope fields.
Later assignments override earlier assignments. Empty lists clear labels, and disabling a filter restores its predecessors.
Artist responses distinguish resolved labels, source evidence, and unresolved conditions.

Album, track, and artist charts constrain contributions before counting and ranking.
Genre constraints combine with date bounds, artist/album constraints, album groups, and undated all-time counts.
The chart dropdown lists published labels. Artist labels link to the corresponding artist chart.
Artist pages prepare or reopen a contextual Naming filter through Edit genres.
Check changes includes before-and-after artist genre sets.

Actual-import acceptance found 10,629 evidence entries linked to 920 catalog artists, producing 178 distinct published labels.
Source records and play fingerprints remained unchanged during chart checks at 390 pixels.
The chart browser check passed without page errors after handling missing image colors.
An isolated manual assignment changed Ra Ra Riot's three source labels to one existing label.
Its preview showed the change, raw records stayed unchanged, and restoring the original configuration restored all three labels.
The main database's previous evidence rebuild preserved 191,473 records, 168,424 plays, and three original-file hashes.

The full Go suite, frontend type checking, and production build pass.
Contextual-editor browser verification passed at 390 and 320 pixels, including draft discard and no page errors.
The main application rebuilt in 63.11 seconds, preserving 191,473 records, 168,424 plays, and all three original-file hashes.
Optional listening tags and replacement of destructive catalog edits remain required Phase 4 work.
