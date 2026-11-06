# Phase 4 requirement audit

Status: **complete**. November 4, 2026.

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
| Case matching also produces clean artist and album statistics | Migration 023, `TestCatalogMatchingTotalsKeepOriginalsAndReverse`, actual artist spellings with 597 plays and album spellings with 1,106 plays | Verified |
| Independent displayed names | Catalog Naming caches and source Naming programs, real-data naming acceptance | Verified |
| Optional regex edition grouping with source, field, artist and release conditions | Typed Grouping schema, grouping/source-exception tests, native incoming-release tests | Verified |
| Grouped reporting retains original releases and recordings | Album report views, group member tables, actual-import grouping checks | Verified |
| Corrected names and credits feed grouping consistently | Shared display environments, contribution-specific artist credits, native refresh and transactional previews | Verified in tests and actual-import acceptance |
| Bulk names, aliases, whitespace/punctuation and artist-credit corrections | Naming operators, catalog alias/credit caches, native search tests, original-preservation checks | Verified |
| Before/after previews and reversible corrections | Shared preview/publication path, savepoint rollback, version restoration and failure regressions | Verified for implemented actions |
| Structured multi-artist credits and names containing commas remain intact | Native history, catalog credits, classification and grouping tests | Verified |
| Explicit corrections survive future imports and rebuilds | Source reimport/rebuild tests, original appearance links, native capture/enrichment regressions and record Naming visibility tests | Verified in tests |
| Source genres remain separate from manual assignments | `source_genres`, derived `artist_genres`, provenance and conflict-selection tests | Verified |
| Artist genres filter charts, including undated totals and date bounds | `TestGenreChartsConstrainCountsBeforeRanking`, actual genre-chart browser checks | Verified |
| Richer source tags remain available without requiring a visualization | Original payloads, raw-field discovery and record inspection | Verified for supported importers |
| Optional concise listening tags | Per-play tag publication, `TestListeningTagsRemainPerPlayAndReverseWithoutChangingRecords`, browser acceptance | Verified |
| Cleanup uses reversible filters rather than destructive merges | Catalog corrections and Inclusion filters, removed destructive routes in `engine/routes.go` | Verified |
| One IF/THEN/ELSE IF/ELSE builder with nested typed operators | Shared operator registry, truth-table tests, migrated filters and browser checks | Verified |
| AND, OR, NOT, NOR, NAND, XOR and XNOR | `internal/filters/program_test.go` truth tables and unknown-value tests | Verified |
| Existing taxonomy, one enabled state, source-aware fields and native music search | Filter manager, schema registry, disabled controls and contextual editors | Verified in prior browser checks |
| Actual imports, repeat/rebuild stability, rollback and originals | Private acceptance logs, 191,473 records, 168,424 plays, three original-file hashes. Final migration preserves twelve original-data and contribution tables | Verified on the closing build |
| Final client typecheck, production build, small-screen checks and GitHub mirror | Full Go suite, client typecheck/build, 390/320-pixel browser checks and GitHub mirror | Verified |

## Referenced issues

- [#359](https://github.com/gabehf/Koito/issues/359) requests case-insensitive grouping for cleaner statistics. Track histories, artist contributions and album reporting now follow comparison values. Reversal restores separate totals.
- [#360](https://github.com/gabehf/Koito/issues/360) requests optional bulk edition grouping that also applies to incoming metadata. Grouping, source exceptions and incoming metadata refresh are implemented.
- [#34](https://github.com/gabehf/Koito/issues/34) requires album distinctions for shared track names and recording IDs. Album appearances are retained. Native ingestion uses original identifiers and preserves recording distinctions after display corrections.
- [#225](https://github.com/gabehf/Koito/issues/225) requests artist genres usable in charts. Source and manual genres, filtering and provenance are implemented. A new genre visualization is outside this phase's written scope.
- [#361](https://github.com/gabehf/Koito/issues/361) supplies the source/metadata tag requirement. Supported imports preserve arbitrary payload fields and optional tags. Remaining ingestion formats belong to their later planned phases.

## Remaining work

None within Phase 4. The implementation and closing verification are mirrored to `custom/koito`.

## Closing matching checks

Migration 023 adds comparison links and reporting caches. Original files, records,
aliases and play/release associations remain independently stored.
Artist and release identifier conflicts stay separate. Source-scoped comparisons
retain their own native/import contexts. Catalog corrections follow their original
appearance links across those contexts.

Manual genres consume combined evidence for a matched artist. Explicit genre
assignments can replace or clear that combined result. Source labels remain available.
Naming stays independent of comparison spelling. Active appearances and explicit
catalog names determine reporting labels. Cached track selections avoid repeating
that choice for every play.

Regression coverage includes:

- `TestCatalogMatchingTotalsKeepOriginalsAndReverse`
- `TestCatalogMatchingConflictingKnownIdentifiersStaySeparate`
- `TestCatalogMatchingIncludesIncomingNativeTracks`
- `TestCatalogComparisonsApplyToSourceTracks`
- `TestSourceScopedCatalogMatchingKeepsIncomingNativeContext`
- `TestNativeEmptyGroupingKeepsMatchedAlbumsSeparate`
- `TestNativeArtistEnrichmentRefreshesMatchedHistoriesAndGroups`
- `TestMatchingKeepsRecordNamingVisibleAfterRebuild`
- `TestLegacyCaseMatchingCleansArtistAndAlbumTotals`
- The existing native recording, multi-artist, operator, import idempotency,
  rollback, genre-chart, grouping and original-preservation suites.

Actual-import case matching combined **Over the Garden Wall** and
**Over The Garden Wall** into an artist with **597 plays**.
The **Fetch The Bolt Cutters** and **Fetch the Bolt Cutters** releases contributed
**17** and **1,089** plays to one **1,106-play** album report.
Original aliases, records and release links retained their hashes.
Restoring the original configuration restored its exact statistics.

These measurements cover the supplied acceptance imports.

The deferred Records styling, alternative missing-album wording and artwork providers remain deferred.
They are not substitutes for the correctness items above.


## Closing verification

All Go packages passed. Client typecheck and production build passed.
The main instance rebuilt 191,473 records into its original 168,424-play view.
All three original-file hashes and the original-record fingerprints matched.
Track 271 retained 521 plays across its 519-play and two-play album appearances.

The closing build verified the 2,807-play grouping example and both mobile widths.
Case matching reproduced the artist and album examples above.
Restoring the original configuration restored its exact statistics and fingerprints.
The original settings are active on both local instances.

The implementation is committed as `b643d95` on `custom/koito`.
Private validation logs remain under `.local`.

Full-data rebuilds and filter changes currently take several minutes on Windows.
The large-import job and progress work belongs to the later operational phase.
