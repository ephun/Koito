# Phase 3: iTunes, inference, and attribution

Phase 3 is in progress.
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
Interface integration remains unfinished.

Private-file acceptance reconciles the XML and converted JSON into one library.
It verifies 14,020 tracks and 47,748 reported cumulative plays across both files.
The expanded file remains a separate set of 56,210 derived records.

## Remaining Phase 3 work

- Integrate the implemented rules into the Filters hierarchy.
- Expose contribution details through contextual record inspection.
- Include undated amounts in suitable totals while preserving calendar boundaries.
- Mark inferred evidence in listening history.
- Provide concise controls and inspectable contributions using the existing interface hierarchy.
- Verify every Phase 3 acceptance criterion against fixtures and private source data.

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
Visible controls, history labels, and aggregate statistics remain unfinished.

## Backend acceptance results

Private-file acceptance passed against 84,495 preserved records.
Original and converted libraries still represent 47,748 reported cumulative plays.
Explicit test attribution yielded 47,084 qualified personal contributions.
Those settings exercised the implementation in an isolated database.
They are not personal attribution defaults.

The test produced 4,592 last-played entries, 41,009 estimates, and 1,480 undated contributions.
Three reported endpoints matched existing inferred library endpoints.
Repeated rebuilding reproduced the same play keys, times, and evidence details.
The raw-record hash remained unchanged.
Disabling Time removed generated events.
Disabling Count removed aggregate contributions.

The full Go suite and focused attribution and inference tests passed.
Frontend controls and native statistics integration remain open Phase 3 requirements.
