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

All library counts and expanded estimates remain evidence during this implementation step.
They produce zero plays until the inference and attribution implementation is ready.

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

The reconciliation engine is currently exercised through automated tests.
Inference rules and interface integration remain unfinished.

Private-file acceptance reconciles the XML and converted JSON into one library.
It verifies 14,020 tracks and 47,748 reported cumulative plays across both files.
The expanded file remains a separate set of 56,210 derived records.

## Remaining Phase 3 work

- Add explicit interpretation choices for decreases, conflicting snapshots, and uncertain identities.
- Add Count, Time, and Context inference as needed by the implemented behavior.
- Add Direct, Heuristic, and Aggregate attribution.
- Preserve unknown attribution and distinguish estimates from observed plays.
- Check attributed estimates against observed listening before counting them.
- Provide concise controls and inspectable contributions using the existing interface hierarchy.
- Verify every Phase 3 acceptance criterion against fixtures and private source data.

Inference options and attribution heuristics still need concrete examples and decisions.
Historical scripts provide evidence for those decisions.
