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

## Implemented storage and filters

- `source_snapshots` stores immutable set metadata separately from its records.
- `source_snapshot_items` retains positions and repeated entries.
- `source_music_states` publishes current memberships without inserting listens.
- `source_release_ids` retains Discogs release distinctions independently of names.
- Playlist manifests preserve removals from later JSON exports.
- CSV playlist filenames distinguish otherwise identical files for different playlists.
- Music matching, Attribution, and Inclusion can read saved, owned, wanted, and playlist evidence.
- Music pages link their status labels to original records. Playlist lists start collapsed.
- Record inspection exposes snapshot metadata alongside the original entry.

Account context includes the account and technical JSON filenames observed in the dump.
These files retain their supplied contents as context rather than playback history.

## Verified actual imports

| Input | Records retained | Repeat upload |
| --- | ---: | --- |
| Spotify account ZIP | 74,389 | All reused |
| Spotify playlist CSV ZIP | 103,422 | All reused |
| Discogs collection ZIP | 278 | All reused |
| Discogs wantlist | 49 | All reused |
| Discogs contributions | 3 | All reused |
| Discogs inventory | 0 | Empty snapshot reused |
| Discogs orders | 0 | Empty snapshot reused |
| Discogs order-items | 0 | Empty snapshot reused |

Together with the previous imports, the working database contains 369,614 records
and 2,316 snapshots. The first successful publication produced 177,692 plays.
There were 5,661 play groups containing both standard and extended Spotify records.
Playlist, saved-library, Discogs, and account-context records contributed zero plays.
Current metadata includes 9,004 saved tracks, 182 saved albums, 278 owned copies,
49 wanted entries, and three contributions. Playlist entries retain multiplicity.

Actual browser checks covered saved tracks, an owned album with zero listens,
native title search, record inspection, editable Completion, and 320px/390px layouts.
Downloading each of the eight originals reproduced its local file's SHA-256 hash.

## Performance verification

The first full publication exceeded a ten-minute client timeout. The transaction
rolled back and retained the previous view and all imported originals.
Profiling a disposable database copy identified repeated membership lookups and
nonselective primary-name joins. Publication now shares item environments and track
lookups. Catalog and history processing read artist credits in batches.
Migration 026 adds covering indexes for primary names.

The corrected full rebuild passed on the copy in 187 seconds and published the
working view in 166 seconds. Its matched-play count remained 177,692.

## Checks and remaining decisions

The Go suite, client typecheck, and production client build passed.
Focused tests cover threshold changes, disabling/removing Completion, precise and
coarse overlap, repeated playlist entries, physical copies, distinct releases,
empty snapshots, removals, original preservation, and atomic archive failures.
The standard-history archive reader rejects malformed named history members.

The defaults use first-seen snapshot ordering. Reuploading an older stored file
does not promote it. Exports without stable playlist identifiers retain their name
and file evidence. JSON and CSV names are not assumed to identify the same playlist.
Saved, owned, and wanted evidence uses supplied track URIs or exact source names.
Catalog Inclusion also follows the shared track history established by Matching.
Further source preference and metadata interpretation remain later-phase work.

The final repeat publication completed in 179 seconds. Fingerprints of records,
snapshots, play identities, contributor links, memberships, release identifiers,
and catalog quantities were unchanged. The final browser recheck also passed.

Phase 5 is complete. See [the closing audit](phase-5-audit.md).
