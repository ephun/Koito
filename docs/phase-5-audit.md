# Phase 5 closing audit

Completed locally on Windows on November 5, 2026.

## Requirement evidence

| Requirement | Evidence |
| --- | --- |
| Finish tag and Edit genres spacing | DOM text baselines align on the actual artist page. Browser checks passed at desktop, 390px, and 320px. |
| Remove separate Records text from listens | Actual listen timestamps open their record inspector. The separate link is absent. |
| Standard Spotify music, podcast, and audiobook JSON | Actual account ZIP imported all three layouts. Unit tests retain nonmusic entries without plays. |
| Preserve minute precision and missing identities | Standard normalization keeps the supplied interval and empty album/URI. Overlap tests assert precision and missing flags. |
| Configurable counting | Qualification → Completion exposes the 30,000ms starting filter. Save, disable, remove, and rebuild tests pass. |
| Standard/extended overlap | The working view contains 5,661 groups with both layouts. Unit tests cover richer representatives and coarse multiplicity. |
| Account and technical context | The reader recognizes observed account/technical JSON filenames. Originals, members, and supplied contents remain stored. |
| Playlist/library JSON and CSV ZIP | Actual imports retained 74,389 account records and 103,422 CSV records. Relevant library sections are retained separately. |
| Identity, positions, repeated entries, dates, IDs, ISRC, other fields | Immutable snapshot metadata and ordered record links retain supplied fields. Raw JSON/CSV values remain inspectable. |
| Snapshot changes and reimports | Empty/removal tests pass. Actual repeats reused every record and snapshot. Older reuploads keep their initial ordering. |
| Membership in filters and the view | Saved, owned, wanted, playlist, and supporting-record fields are available to Matching, Attribution, and Inclusion. Browser/API checks pass. |
| Discogs collection ZIP/CSV, wantlist, contributions | Actual imports retained 278 collection records, 49 wanted records, and three contributions. |
| Inventory, orders, order-items, including headers alone | All three actual header-only exports were accepted as zero-record snapshots. |
| Physical copies and release distinctions | Repeated-copy and distinct-release tests survive repeat rebuilding. The view retains 278 collection entries. |
| Conditions, notes, formats, labels, catalog numbers, dates, unknown years | Original CSV columns and values remain stored. Release projection does not assign an unknown year of zero. |
| Unnamed wantlist value | Inspection retains nine columns and ten values. The extra timestamp has no invented field meaning. |
| Metadata does not manufacture listening | Zero playlist, library, Discogs, or account-context records contributed plays. A real owned album has zero listens. |
| Native search | The real owned album was found through the existing title-search endpoint. |
| Clean layout | Playlists start collapsed. Long names fit when expanded on mobile. Unplayed pages omit empty listening charts. |
| Original file preservation | All eight downloads match their local original SHA-256 hashes. The original record prefix matches its pre-Phase 5 database backup. |
| Failure and retry | Archive-validation tests reject malformed standard and extended history members atomically. Projection failure tests preserve prior results. |
| Repeat publication | Full table fingerprints and catalog quantities stayed unchanged across another working rebuild. |

## Working data

- 369,614 immutable records and 2,316 snapshots.
- 177,692 plays with zero collisions.
- 9,004 saved tracks and 182 saved albums.
- 278 owned entries, 49 wanted entries, and three contributions.
- 129,579 current playlist entries, including supplied repetitions and distinct export evidence.
- Metadata imports produce zero inferred or undated plays with the current settings.

These counts come from actual imports. Unit fixtures were not seeded into the working application.

## Closing checks

The applicable full Go suite, client typecheck, and production client build passed.
Actual-browser verification covered the filters page, track/album pages, record
inspection, native search, original downloads, and small-screen layouts.
The original genre/timestamp checks passed again after importing the new data.

The corrected database-copy rebuild passed in 187 seconds. Working publications
took 166 and 179 seconds. The repeated publication retained the same records,
snapshots, play identities, supporting links, music states, release identifiers,
and catalog quantities.

Private logs and screenshots are under `.local`. Export contents, credentials,
databases, profiles, and screenshots are excluded from Git.

## Decisions carried forward

Completion thresholds are explicit editable filters. Missing completion flags
remain missing. Coarse repeated timestamps retain their uncertainty.
Playlist identities without supplied IDs retain filename/name evidence. Cross-format
names are not treated as proof of a shared playlist. Current snapshots use stable
first-seen ordering. Explicit source preference and broader metadata interpretation
remain later-phase work.

The branch remains `custom/koito`. Production deployment belongs to Phase 10.
