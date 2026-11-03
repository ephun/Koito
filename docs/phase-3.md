# Phase 3: iTunes, inference, and attribution

Phase 3 implementation and acceptance checks are complete.
The [development plan](development-plan.md#phase-3-itunes-inference-and-attribution) defines its full scope.
The existing filter taxonomy remains authoritative.

## Imports implemented

- Original iTunes plist XML.
- Converted library JSON arrays, including scalar fields wrapped in arrays.
- Expanded JSON arrays containing Estimated Play Time.
- ZIP uploads containing these formats.

Original bytes remain in file storage.
XML extraction produces a library header, track records, and whole playlist records.
Track records retain the library identity, snapshot date, and original dictionary key when supplied.
Converted arrays retain their supplied fields without inventing a library identity or snapshot date.
Playlist records retain ordering and repeated entries.

Repeated XML dictionary values use an explicit `$plist_repeated` wrapper.
This distinguishes repeated keys from source arrays.
Unknown metadata and sentinel dates remain inspectable.
Malformed extraction retains the file and records the failed upload.

Library counts and expanded estimates remain evidence by default.
Saved Count, Time, and Attribution rules can interpret library counts.
The historical expanded rows remain separate from new inference.

## Verification

Automated fixtures cover XML, JSON, repeated fields, large integers, playlist repetitions, malformed input, and archive identity.
Rebuild checks confirm that library evidence stays separate from observed plays.

Private-file acceptance uses an isolated temporary database.
Run it with `KOITO_TEST_ITUNES_DIR` pointing to the original library directory.
The command is `go test ./internal/sources -run TestLocalPhase3Libraries -v`.
Private files remain outside Git.

## Count reconciliation implemented

Library identity and persistent track identity separate independent libraries.
Converted copies link only when their complete track fields corroborate one known library.
Ambiguous copies retain unknown identity.
Library fingerprints account for the historical conversion's scalar strings, arrays, booleans, and repeated-field behavior.
Preserved records retain their original representations.

Each track retains every supporting count snapshot.
Its reported amount uses the largest valid cumulative count.
Later snapshots contribute evidence about that amount rather than another lifetime total.
The latest dated reported count remains separate from the maximum.
Missing counts, zero counts, and invalid counts remain distinct.
Unknown persistent identities retain their reported amounts outside the combined library total.

Decreases, conflicting counts, unknown snapshot order, repeated identities, and changed metadata remain explicit issues.
Dates retain sentinel, invalid, reversed, and post-snapshot conditions for inspection.
These issues require interpretation before affected estimates can enter the view.
Snapshot order uses supplied dates rather than upload order.

The reconciliation and inference engines are exercised through automated tests.
Filters, contextual inspection, history labels, and all-time statistics expose their results.

Private-file acceptance reconciles the XML and converted JSON into one library.
It verifies 14,020 tracks and 47,748 reported cumulative plays across both files.
The expanded file remains a separate set of 56,210 derived records.

## Interface and totals

Filters uses the existing Matching, Inference, and Attribution hierarchy.
Count and Time controls appear when library records are present.
Direct, Heuristic, and Aggregate attribution rules are added explicitly.
Scope conditions appear on demand.
Disabled rules retain their settings and disable their controls.
Library timing depends on enabled Count rules.
Hover and keyboard help use short sentences.
Escape dismisses help during hover or focus.

One Save changes action publishes the view.
Discard restores saved settings.
Incomplete scope conditions require completion or removal before saving an enabled rule.
Stored versions remain available internally.

Record inspection shows reported, interpreted, attributed, observed, estimated, and undated amounts.
Supporting records and date intervals remain available there.
History labels identify estimated times, reported endpoints, and heuristic attribution.

Undated amounts contribute to all-time play totals and rankings.
Their tracks, albums, and artists contribute to all-time counts.
Date ranges, calendar activity, streaks, and daily averages retain dated listening semantics.
Duration totals retain dated event semantics.
The dashboard shows the undated portion when present.
Imports includes undated amounts in its source totals.
Statistics cache keys include the current view publication.

## Attribution and inference backend

The backend supports these existing rule types and subtypes.

| Type | Subtype | Behavior |
|---|---|---|
| Inference | Count | Choose maximum, latest, or explicitly interpreted reset totals. |
| Inference | Time | Use reported last-played evidence or distribute remaining estimates across supporting intervals. |
| Attribution | Direct | Assign personal, other, or unknown listening. |
| Attribution | Heuristic | Require matching personal plays from an explicitly selected evidence source. |
| Attribution | Aggregate | Assign an explicit percentage of cumulative counts. |

New rules remain opt-in.
Existing Spotify and Last.fm history retains its previous treatment until attribution rules apply.
Library attribution starts unknown.
Heuristic evidence requires explicit personal attribution on its supporting source.
Repeated exports contribute distinct records but one matched supporting play.
Heuristic results retain their rule and basis.
Aggregate percentages round down to whole plays.
Partial attribution identifies an amount rather than specific observed events.

Scopes support service, source, file, library, persistent track, record, artist, device, and explicit date bounds.
Device scope uses supplied device fields or Spotify's platform field.
Library date scopes must contain the entire supporting interval.
Record rules take precedence, followed by track and file scopes.
Other scopes combine source, library, device, artist, and date specificity.
Equal-specificity contradictions remain unknown.
Explicit unknown attribution takes precedence over heuristics.

Count modes preserve reported amounts separately from interpreted totals.
The reset interpretation requires dated, consistent snapshots.
Its additional counts remain undated while segment timing remains uncertain.
Unidentified libraries require an explicit file scope before counting.
Unknown track identities remain unresolved.
Nonmusic and unidentified music retain their evidence outside counted music contributions.

Observed personal plays reserve their contributions before time estimation.
One observed play covers at most one cumulative contribution.
Equal reported last-played endpoints can corroborate an already represented event when play matching is enabled.
Estimated positions cannot establish that independent cumulative counts describe the same plays.
Estimates avoid occupied source timestamp intervals, including minute precision.
Each contribution retains its supporting records, intervals, attribution, applied rules, and undated remainder.
A per-rebuild generation bound leaves excess amounts undated.

Saving publishes rules, event contributions, and aggregate contributions in one transaction.
Rebuilding retains deterministic play keys and times for unchanged evidence and settings.
Failed publication leaves previous rules and contributions intact.
Disabling inference removes its contributions while preserving files and records.
Schema rollback follows the existing requirement to restore a pre-migration backup.

The backend supports contextual contribution inspection and composite cursors for inferred events sharing one record.
The interface exposes these contributions through the existing Filters and Imports pages.

## Backend acceptance results

Private-file acceptance passed against 84,495 preserved records.
Original and converted libraries still represent 47,748 reported cumulative plays.
Explicit test attribution yielded 47,084 qualified personal contributions.
Those settings exercised the implementation in an isolated database.
They are not personal attribution defaults.

The test produced 4,592 last-played entries, 41,039 estimates, and 1,450 undated contributions.
Three reported endpoints matched existing inferred library endpoints.
Repeated rebuilding reproduced the same play keys, times, and evidence details.
The raw-record hash remained unchanged.
Disabling Time removed generated events.
Disabling Count removed aggregate contributions.

The full Go suite and focused attribution and inference tests passed.
Client type checks, the production build, and Chromium and Firefox acceptance passed.
Browser checks cover 320-pixel and 390-pixel layouts, long filenames, scoped rules, help, and persisted settings.

## Historical script evaluation

The earlier expansion applies a minimum count of one to missing or zero counts.
That assumption explains the larger derived export.
Original library counts and expanded rows remain separate in this fork.

The old iTunes thinning formula uses a fixed cap below its comparison threshold.
It switches to a source ratio above that threshold.
For example, 100 target plays and nine comparison rows permit 40 target rows.
Ten comparison rows instead permit 20 target rows.
That discontinuity makes the historical formula unsuitable as an automatic default.
The comparison source list also includes playlist entries.
Membership records supply context for later attribution work.

The thinning expression references a discarded `n_target` column.
Its row numbers span the source rather than each track.
Fractional selection sequences also complicate the intended retained amount.
These details support treating the scripts as examples rather than importing their output as established attribution.

Current rules use explicit personal shares and explicitly attributed evidence sources.
Historical source ratios and caps remain personal choices for later refinement.

## Phase 3 acceptance audit

| Requirement | Evidence |
|---|---|
| Original files, metadata, playlists, and repeated imports | `itunes_test.go` and private-file hash comparisons |
| Snapshot identity, copies, resets, invalid dates, and unknown counts | `library_test.go` and `inference_test.go` |
| Separate totals, endpoint use, and time estimation | Filters browser acceptance and inference tests |
| Direct decisions by source, device, date, and record | `attribution_test.go` and scoped browser controls |
| Heuristics and partial aggregate attribution | Attribution and inference tests, plus browser acceptance |
| Deterministic estimates and observed overlap | Inference tests, interval boundary tests, and private-file rebuild hashes |
| Preserved evidence after disabling rules or publication failures | Source preservation and transaction failure tests |
| Undated totals, rankings, and calendar boundaries | `counted_listening_test.go` and dashboard browser checks |
| Reported counts remain distinct from expanded rows | Private-file acceptance verifies 47,748 reported plays and 56,210 derived rows |
| Minimal interface, hover help, and small screens | Chromium and Firefox acceptance at desktop, 390 pixels, and 320 pixels |

The implemented controls are ready for personal preference review.
Library attribution starts unknown.
New inference and attribution rules start disabled.
Personal shares and evidence sources take effect through saved rules.
