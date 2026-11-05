# Phase 4 requirement audit

Status: **in progress**. November 4, 2026.

The scope remains [Phase 4 in the development plan](development-plan.md#phase-4-music-matching-album-distinctions-and-bulk-cleanup).
This audit distinguishes implemented behavior from remaining correctness work.
Passing a narrower test does not close a broader requirement.

| Requirement | Current evidence | Result |
|---|---|---|
| Additive catalog migration with retained original track/listen links | Migration 008, `internal/music/identity.go`, `TestMusicHistorySpansAlbumsWithoutCombiningPlays` | Verified |
| Track pages span album appearances while each play keeps its album | `internal/db/sqlite/music_test.go`, actual track 271 with 519 and 2 plays on separate albums | Verified |
| Recording identity remains distinct from name-based history | `TestNameHistoryKeepsRecordingVariantsDistinct`, `TestMusicAliasKeepsDisplayAndRecordingIdentitySeparateAndReverses` | Verified for imports and ordinary native capture |
| Native capture preserves different recording identifiers even after display corrections | `TestDisplayedRecordingIdentifierDoesNotReidentifyIncomingNativePlays`, original-read regressions and corrected isolated reproducer retain both IDs | Verified in tests and activated locally |
| Singles, compilations, missing albums, remasters, live/acoustic versions, untitled tracks | `TestLiveRecordingKeepsEachSubmittedAlbum`, `TestNameHistoryKeepsRecordingVariantsDistinct`, edition-template tests | Verified |
| Case-sensitive and insensitive comparisons preserve original capitalization | Music program migration, source comparison overrides, Unicode case/history tests | Verified for track histories |
| Case matching also produces clean artist and album statistics | Isolated case-insensitive fixture retains two artists and two albums while sharing one track history | **Needs work** |
| Independent displayed names | Catalog Naming caches and source Naming programs, real-data naming acceptance | Verified |
| Optional regex edition grouping with source, field, artist and release conditions | Typed Grouping schema, grouping/source-exception tests, native incoming-release tests | Verified |
| Grouped reporting retains original releases and recordings | Album report views, group member tables, actual-import grouping checks | Verified |
| Corrected names and credits feed grouping consistently | Current worktree adds shared display environments, source refresh and exact projected previews. New source and native regressions pass | Verified in tests and current-build real-data acceptance |
| Bulk names, aliases, whitespace/punctuation and artist-credit corrections | Naming operators, catalog alias/credit caches, native search tests, original-preservation checks | Verified |
| Before/after previews and reversible corrections | Shared preview/publication path, savepoint rollback, version restoration and failure regressions | Verified for implemented actions |
| Structured multi-artist credits and names containing commas remain intact | Native history, catalog credits, classification and grouping tests | Verified |
| Explicit corrections survive future imports and rebuilds | Source reimport/rebuild tests and native capture/enrichment regressions | Verified in tests |
| Source genres remain separate from manual assignments | `source_genres`, derived `artist_genres`, provenance and conflict-selection tests | Verified |
| Artist genres filter charts, including undated totals and date bounds | `TestGenreChartsConstrainCountsBeforeRanking`, actual genre-chart browser checks | Verified |
| Richer source tags remain available without requiring a visualization | Original payloads, raw-field discovery and record inspection | Verified for supported importers |
| Optional concise listening tags | Per-play tag publication, `TestListeningTagsRemainPerPlayAndReverseWithoutChangingRecords`, browser acceptance | Verified |
| Cleanup uses reversible filters rather than destructive merges | Catalog corrections and Inclusion filters, removed destructive routes in `engine/routes.go` | Verified |
| One IF/THEN/ELSE IF/ELSE builder with nested typed operators | Shared operator registry, truth-table tests, migrated filters and browser checks | Verified |
| AND, OR, NOT, NOR, NAND, XOR and XNOR | `internal/filters/program_test.go` truth tables and unknown-value tests | Verified |
| Existing taxonomy, one enabled state, source-aware fields and native music search | Filter manager, schema registry, disabled controls and contextual editors | Verified in prior browser checks |
| Actual imports, repeat/rebuild stability, rollback and originals | Private acceptance logs, 191,473 records, 168,424 plays, three original-file hashes | Verified at the last committed activation. Repeat after remaining fixes |
| Final client typecheck, production build, small-screen checks and GitHub mirror | Earlier checks pass. Final checks and push must cover the closing revision | Pending |

## Referenced issues

- [#359](https://github.com/gabehf/Koito/issues/359) requests case-insensitive grouping for cleaner statistics. Track-history comparison works. Artist and album totals remain a required gap.
- [#360](https://github.com/gabehf/Koito/issues/360) requests optional bulk edition grouping that also applies to incoming metadata. Grouping, source exceptions and incoming metadata refresh are implemented.
- [#34](https://github.com/gabehf/Koito/issues/34) requires album distinctions for shared track names and recording IDs. Album appearances are retained. The native submission/display-identifier interaction still needs repair.
- [#225](https://github.com/gabehf/Koito/issues/225) requests artist genres usable in charts. Source and manual genres, filtering and provenance are implemented. A new genre visualization is outside this phase's written scope.
- [#361](https://github.com/gabehf/Koito/issues/361) supplies the source/metadata tag requirement. Supported imports preserve arbitrary payload fields and optional tags. Remaining ingestion formats belong to their later planned phases.

## Remaining work

1. Make artist and album statistics follow configured music identity matching while preserving raw entities and source associations.
2. Verify the resulting behavior against this complete matrix, run closing checks, and push the final revision.

The deferred Records styling, alternative missing-album wording and artwork providers remain deferred.
They are not substitutes for the correctness items above.
