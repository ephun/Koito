# Phase 5: Spotify, playlists, saved music, and Discogs

Phase 5 adds the remaining Spotify export layouts and non-play music evidence.
Original files and records stay separate from the derived view.

The spacing correction is complete. Genre text and Edit genres share a baseline.
Listen timestamps open their records. Commit `02b69f7` includes the correction.
Browser checks used actual imported pages at desktop, 390px, and 320px widths.

## Requirements and verification

| Area | Required behavior | Evidence required |
| --- | --- | --- |
| Standard history | Music, podcast, and audiobook layouts from the dump | Actual files imported and repeat imports checked |
| Precision | Keep minute timestamps, missing IDs, and original milliseconds | Normalization and overlap tests |
| Completion | Configurable under Qualification, default 30 seconds for account history | Save, disable, remove, and rebuild checks |
| Overlap | Standard and extended histories describe one play when uniquely matched | Synthetic ambiguity tests and actual export comparison |
| Account ZIP | Preserve original ZIP, every member, and account context | Byte hashes and record inspection |
| Playlists | JSON and CSV, identity evidence, positions, repeated tracks, dates, and extra metadata | Real imports plus repeated-entry tests |
| Library | Saved tracks, albums, artists, shows, episodes, concerts, and other sections | Actual section coverage |
| Snapshots | Reimports reuse snapshots, later exports replace membership rather than append | Snapshot and empty-export tests |
| Discogs | Collection, wantlist, contributions, inventory, orders, and order-items | Real CSVs including header-only exports |
| Copies | Preserve multiple physical copies and release IDs | Repeated-row and changed-snapshot tests |
| Metadata | Preserve conditions, notes, format, label, catalog number, dates, unknown years, extra unnamed values | Raw-record assertions |
| View | Saved, owned, wanted, and playlist evidence available without increasing plays | API, filter, and browser verification |
| Closing checks | Go tests, client typecheck/build, actual import acceptance, GitHub mirror | Current logs and pushed commit |

## Counting defaults

Spotify account history contains played milliseconds without completion flags.
Its initial Completion filter counts identified music with at least 30,000 milliseconds played.
This is an editable IF condition and counting action in the existing filter manager.
Disabling or removing it restores counting of valid music playback observations.
Extended history keeps its existing reported-complete default.
Completion filters can explicitly change that decision using the supplied fields.
Invalid timestamps, invalid durations, and non-play records remain separate evidence.

Coarse timestamps cannot establish that two identical rows represent one playback.
Multiplicity within a minute remains ambiguous rather than collapsing genuine repeats.
When standard and extended records match, the extended record supplies richer display metadata.

## Snapshot design

Snapshot identity and membership are separate from listening.
Each snapshot points to its original file and immutable source records.
Membership retains positions and repeated entries.
Header-only exports can record empty snapshots without creating an item.
The same import reuses its snapshot and keeps its original position in snapshot history.
Selecting the current snapshot does not turn added dates or purchase dates into listen dates.
Release and copy evidence remains available even when displayed names match.

Progress is recorded through implementation and acceptance results. Phase 5 remains in progress.
