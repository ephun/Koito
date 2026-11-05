# Spotify coverage and interface follow-up

Verified locally on November 5, 2026, after the Phase 6 audit.

## Missing historical imports

The main development database contained the 2026 extended history and a recent standard Spotify account export. The full personal archive had not been imported there. Its 2015 through 2025 history explains the earlier total of about 17,900 Spotify plays.

The original ZIP archives were imported through the existing source importer:

| Source | History files | History records | Export date range |
| --- | ---: | ---: | --- |
| Personal Spotify history | 37 | 340,462 | January 10, 2015 through October 19, 2026 |
| First family Spotify history | 8 | 119,611 | July 15, 2011 through January 2, 2025 |
| Second family Spotify history | 8 | 108,938 | May 15, 2012 through January 17, 2025 |

The personal import reused 15,563 existing records and added 324,899. Each archive was imported again. Every repeat added zero records and reused its complete history.

The database now holds 1,096,629 source records across all services. The personal Spotify source has 518,273 records, including previously imported playlists, library information, and standard history.

The final published view contains 264,151 plays across services. The personal Spotify source supports 173,395 of those plays. Source totals overlap when multiple services describe one play, so they should not be added together.

## Completion and listening time

The personal extended archive contains 336,805 identified music records. Of those, 165,294 report `reason_end=trackdone`. There are 209,006 identified music records with at least 30 seconds played. These measures describe different sets of records.

The existing Completion defaults remain in effect. Extended history uses its reported completion information. Standard account history uses the played-duration condition. Both can be changed through Qualification filters.

The raw personal archive supports the expected annual listening scale. Summing its supplied `ms_played` values gives approximately 77,948 minutes in 2019, 90,930 in 2020, 84,807 in 2021, and 78,923 in 2022. These totals include partial listening. They are not counts of completed plays or reproductions of Spotify Wrapped.

## Attribution

Family histories have separate source identities. Direct Attribution filters leave family listening unassigned while preserving every record. Another Direct Attribution filter identifies the existing personal Last.fm, Spotify, Navidrome, and Multi-Scrobbler histories as personal evidence.

When a family record matches an independently recorded personal play, it can support that same play. A family record alone contributes no personal play. Shared-account heuristics remain later work.

## Preservation checks

All three original ZIP files match their retained bytes. All 53 history members match their stored checksums and expected record counts. The first 369,614 pre-existing records match the checksum recorded before Phase 6. SQLite foreign keys pass.

Original files remain under the local source store, separate from the database projection. None of these private files are committed to GitHub.

## Interface changes

- Settings no longer contains About. The sidebar link to koito.io is removed. The footer remains.
- Filters uses a funnel icon.
- The Imports inspection heading and Records buttons share the same horizontal center.
- Artist, album, and track headers share their art, title, statistics, and metadata layout.
- Artist links use lowercase `by`.
- Genres and source tags share a chip component and consistent spacing.
- Playlists opens a positioned, scrollable menu. It closes with Escape or an outside click.
- Named sources appear distinctly in source selectors without adding an upload account control.
- The unused update check for the removed About controls is deleted.
- Imports summaries read the compact record/source index once and retain compatibility with older contributor links.

The iTunes import retains 244 playlist records, their fields, and track references alongside the original XML. Those iTunes memberships are not yet projected into the Playlists menu.

## Verification

The full Go suite, client typecheck, and production build pass. Browser checks cover artist, album, and track pages at 1280, 768, 390, and 320 pixels. Opening Playlists leaves art, title, and statistics bounds unchanged. Settings and sidebar checks confirm both About controls are absent and the footer remains.

Source identity and attribution tests verify separate family histories, one Spotify service choice, corroborated personal listening, and exclusion of family-only listening.

The summary rewrite matches every source identity, record count, play count, undated count, and date range from the previous query against the full database. The new Go implementation took 17.5 seconds in that local read-only check while the rebuild was running. Its regression test also checks duplicate supporting records, unassigned family records, cumulative counts, partially populated contributor links, and older projections.

The final local rebuild took 1,109 seconds and published 264,151 plays atomically. After restarting with the final code, the Imports API returned in 28.4 seconds. Large-history rebuild and summary performance remain areas for further improvement.
