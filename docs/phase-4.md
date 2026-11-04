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

1. Implement typed, atomic values, comparisons, operators, branches, and actions.
2. Migrate existing configurations while retaining their behavior and disabled settings.
3. Replace the mixed filter controls with the consistent IF/THEN/ELSE builder and manager.
4. Add reversible music aliases, source-specific naming, edition grouping, and bulk cleanup.
5. Show affected examples for pattern edits and preserve explicit exceptions and undo.
6. Add source and manual genres with artist genre assignment and chart filtering.
7. Add optional source and metadata tags to listening rows.
8. Integrate contextual corrections with native search and audit the remaining legacy editing operations.
9. Verify every Phase 4 acceptance criterion against the completed implementation.
