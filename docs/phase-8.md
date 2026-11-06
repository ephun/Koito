# Phase 8: live capture and remaining filter controls

Status: local implementation and validation in progress. Server connection and
current-server backup still require commands run by the user. The acceptance requirements remain those in
[the development plan](development-plan.md#phase-8-live-capture-and-remaining-filter-controls).

## Live intake

ListenBrainz submissions enter a disk journal before acknowledgment. The journal
references a synced, content-addressed copy of the exact request bytes. It stores
the receiving user, reception time, request identifier, and user agent separately.
Authentication credentials are excluded.

The journal remains writable while a view transaction holds SQLite's writer.
The worker registers accepted submissions and their complete payload records,
then uses the existing matching, attribution, inference, and publication path.
Publication failure leaves accepted evidence and pending journal entries intact.
Restart recovery replays pending entries without importing them twice.

An explicit `Idempotency-Key` with identical bytes reuses records while retaining
both delivery attempts. A reused identifier with different bytes preserves the
new input. Deliveries without an explicit retry identifier retain their record
occurrences. Play matching interprets their potential overlap.

Now-playing records remain separate from completed plays, including when they
contain a timestamp. Playback state reads committed submissions and the pending
journal, supports expiry and stop updates, and survives process restarts.

Configured ListenBrainz relays use an independent durable retry path. Retries
send the original bytes with a stable delivery identifier. Publication can finish
while the destination is unavailable, and relay completion survives restart.

## Recent listening

The existing November 2 Koito backup is the historical baseline. The user
confirmed it ends several days before this phase. Recovering subsequent listening
is part of this phase.

Original Multi-Scrobbler API responses were preserved locally on November 6.
Their manifest records response hashes. The new API-page reader retains original
rows, source event identifiers, and destination attempts. The saved pages are
private and excluded from Git.

The Multi-Scrobbler container on PhunRAID successfully reached the development
fork through Tailscale and validated its dedicated token. Live delivery has not
been configured. Existing Last.fm and production Koito destinations remain active.

A continuous local capture saves complete Last.fm history pages from November 1
and periodic Multi-Scrobbler API pages. These originals include overlap with the
baseline. The selected backfill added 1,590 records to the working database.
Repeating those imports added zero records. The working view rebuild published
265,537 plays from 1,106,424 records in 901 seconds. It reported zero collisions.
A comparison with the pre-intake database found zero changed or missing baseline
records and zero missing or size-changed file registrations. Saved filter version
9 remained selected. Record counts are not unique-play counts.

A separate integrity pass verified all 1,422 registered original files against
their SHA-256 hashes, covering 3,612,629,613 bytes. No files were missing or damaged.
The November 6 coverage check found every row in the latest complete Last.fm
capture (221 completed rows) and Multi-Scrobbler capture (1,354 retained rows)
already present in the working records. This proves captured-source coverage,
not listening omitted by upstream services or a configured live destination.

The installed Multi-Scrobbler version is 0.17.2. Its ListenBrainz conversion
forwards service, player, recording identifiers, and recording duration. It omits
native event identifiers, device identifiers, and actual listened duration.
Retaining its API pages preserves richer source evidence alongside submissions.
The fork cannot reconstruct fields omitted from an incoming request.

`scripts/backup-multi-scrobbler.cjs` creates a SQLite online backup of `ms.db`,
copies configuration, credential files, and logs, and writes an integrity-checked
hash manifest. It runs in Multi-Scrobbler's Node environment and keeps the service
running. The database snapshot includes committed history and persisted queues.
Other files are copied while the service runs and do not share a database
transaction. Existing `.bak` files remain in their original location.

The script passed against the actual saved Multi-Scrobbler appdata from November
2, including its persisted queue table. The user also ran it successfully in the
current `multi-scrobbler` container on November 6 at 20:25 UTC. It verified seven
files in `/config/koito-phase8-backup-20261106T202519Z`. The active configuration
matches the saved configuration hash. SMB exposes appdata for reading but denied
writes and access to the private backup folder. Backup permissions remain unchanged.

The saved Compose file mounts `/mnt/cache/appdata/multi-scrobbler/config` at
`/config`. Every configured source has an explicit destination list. Adding a
client alone would leave it unused. `scripts/connect-multi-scrobbler.cjs` verifies
the backup, validates the receiving token, adds the development client, and extends
the lists that currently include Koito. It preserves existing destinations and
checks for concurrent config changes before an atomic replacement. Tests include
the actual saved configuration, a real local token preflight, and repeated setup.
The server configuration has not yet been changed.

`scripts/backup-koito.cjs` creates a verified online snapshot of the original
Koito database and copies its configuration files. It passed against the saved
local Koito baseline. Running it against the original server remains pending.

## Filter controls and native pages

Qualification now includes Completion, Repetition, and Validity in the existing
IF/THEN editor. Reported played time, recording duration, skips, and completion
remain separate fields. Missing facts remain unknown. Repetition examines distinct
plays after matching and attribution. Count inference reserves observed evidence
before repetition exclusions, so excluded repeats do not return as estimates.

Inclusion supports evidence level and attribution conditions. Evidence-level
conditions apply to dated plays and undated cumulative counts. Undated counts
remain separate from dated plays, retain their original contributions, and pass
through the same all-time count views used by track, artist, and album reports.
Disabling an inclusion filter restores its excluded listening.

Manual web submissions preserve their original request and selected track
snapshot before entering the native intake journal. Dated-play exports use
displayed listening. Their existing format retains original aliases and omits
undated counts. The Export hover text explicitly describes this format difference.
Original files remain available through Imports.

Entry-page typography and layout use stock Koito classes again. Playlist
membership is hidden from entry pages while imported memberships remain stored.
The user's saved personal filter configuration has not been replaced.

Backend tests and client type checking/build pass at this checkpoint. The latest
local receiver runs migration 30. Browser checks verified stock entry-page title,
cover, rank, and spacing classes at desktop and mobile widths. Filter headings,
the repetition draft editor, mobile width, and unchanged saved version also pass.

Browser checks also verified now-playing rendering, unlinked music without a
catalog identifier, and removal after a stop update. These use browser-only
responses and add no synthetic listening to the working database. Imports and
Filters have matching page spacing at desktop and mobile widths. Pending page
navigation has a loading indicator. Artist, album, track, album-group, and Rewind
loaders cancel requests when another navigation supersedes them.

Working-data recovery and the real sender connection remain in progress.
Continuous history capture remains running. Broad query and rebuild performance
work belongs to Phase 9. The GitHub branch remains a development checkpoint.

### Publication performance checkpoint

A controlled rebuild of a disposable working-database copy took 770 seconds
before optimization and 623 seconds afterward. Both runs processed 1,106,424
records into 265,498 plays. Hash comparisons of 17 derived tables were identical,
including interpretations, contributing records, attribution evidence, and scope.
This establishes equivalent output for that comparison, with about 19% less time.
It does not establish acceptable per-listen latency.

The changes remove duplicate decoding and repeated attribution environments,
reuse transaction-local SQL statements, avoid repeated evidence serialization,
and temporarily enlarge the writer's SQLite page cache. A test verifies that the
cache setting is restored after commit and failed publication. Later small
allocation reductions are tested but were not included in the timed comparison.

Explicit retries of already-published records now finish without another full
rebuild. Their delivery attempts and publication version remain recorded.
Unidentified repetitions still retain new records and require interpretation.
Tests also cover distinct native event identifiers at the same second, including
integers above JavaScript's exact-number range, using an explicit matching filter.

The current rebuild count differs from the earlier working publication because
the API reader now honors Multi-Scrobbler's source-duplicate state, consistently
with its database reader. Its original rows remain stored. Personal filter
version 9 is unchanged. This checkpoint has been published to the working
database. The following incremental checks measure the routine live-update path.

### Incremental live publication

Live intake can now recalculate a connected music history instead of replacing
the entire view. Its dependency index includes matching names, recording IDs,
native event IDs, exact source copies, supporting deliveries, manual matches,
and ambiguous candidates. It keeps records belonging to other listeners in the
dependency set so attribution and matching can be reconsidered together.

The same calculation runs for full and incremental publication. A comparison
on the disposable real-data database matched all 33 checked derived tables
after a fresh full rebuild. Unit comparisons also cover same-second native
events, direct attribution, excluded listeners, repetition, inclusion, shifted
and estimated timestamps, gameplay context, manual matches, and rollback.

Current incremental publication uses existing catalog appearances. New catalog
metadata, naming/grouping changes, library count inference, heuristic attribution,
structured concert revisions, changed snapshots, and large dependencies use the
full path. This is a performance fallback. All filters still run normally.

Forwarding records with absent music names previously shared an empty evidence
key. The cache repair removes that association while retaining original records
and their parent-play links. Explicit comparison assignments for identified
music keep their existing matching semantics.

The final timed replay reconsidered 86 real records and 41 plays in 11.9 seconds.
Its journal acknowledgment took 6 milliseconds. Repeating the preserved listen
kept the play count unchanged. An interrupted accepted request also recovered
successfully on the disposable copy. These were local replays, not proof of
delivery from the running Multi-Scrobbler server.

The working database's latest full rebuild completed in 602 seconds with
1,106,424 records, 265,498 plays, and saved filter version 9. The latest receiver
uses migration 30. Its dependency-index warmup completed in 174 seconds, indexing
480,231 eligible records and 758,148 music keys. Saved filter version 9 remains
selected. The full Go suite, client type checking, client build, and browser
checks pass at this checkpoint.

An additional integration test imports Spotify and Last.fm records and captures
a native listen. Source/date qualification, displayed date ranges, and attribution
basis conditions produce consistent history, totals, charts, activity, and dated
exports. Disabling those filters restores the listening. Original records remain
present throughout. Existing catalog tests verify names and identities in native
search. The catalog remains searchable when listening is excluded.

### Capture recovery

The temporary history-capture process stopped after a disk-full error. Capture
was restarted after space became available. It now writes response files
atomically and retries status-write failures. The recovery poll again fetched
the complete Last.fm window beginning November 1. Current results still contain
221 rows. Earlier Multi-Scrobbler responses remain preserved even when its live
API retention removes older rows. The original server destinations remain active.

Further actual listening arrived during validation. The latest completed captures
were imported with overlap, including changed source rows as separate evidence.
The catch-up view rebuild is in progress. This batch does not change saved filters.

## Acceptance audit

| Requirement | Evidence still required before completion |
|---|---|
| Preserve complete accepted bytes and unknown fields | Unit and HTTP checks pass. Verify deployed receiver with real submissions. |
| Retry and restart recovery | Unit checks pass for explicit retry keys, publication failure, and restart replay. Verify actual sender retry behavior. |
| Separate now-playing from completed plays | Unit checks cover timestamped updates, expiry, stop, ordering, completion, and restart. Browser rendering checks pass. Verify actual sender behavior. |
| Preserve source, device, app, and native identifiers | Initial ListenBrainz fields retained and interpreted. Verify Multi-Scrobbler's actual outgoing fields and distinct event behavior. |
| Preserve current history, database, queues, and configuration | Current Multi-Scrobbler online backup verified by the server-side helper. Current Koito online snapshot remains pending. |
| Backfill since the last export with overlap | Imported 1,590 actual records. Reimport added zero. Rebuild and baseline-record comparison pass. Compare coverage with current destinations and investigate gaps. |
| Add the fork as another destination | Connectivity and authentication verified. Configure and observe real delivery after receiver verification. |
| Remaining qualification and inclusion controls | Completion, played time, media/app facts, source/date qualification, date ranges, attribution basis, and evidence-level conditions pass integration tests. Undated inclusion passes. Inspect real connected submissions. |
| Repeat limits after matching and attribution | Distinct plays, supporting records, other listeners, time windows, and count-inference reservations pass the repetition tests. |
| Consistent history, search, statistics, charts, and exports | Cross-view integration tests pass. Catalog tests cover native search. Export hover text explains its original-alias and dated-only format. |
| Existing native behavior | Finish integration checks, preserve manual submissions, and verify any configured relay behavior. |
| Working-data performance and recovery | Measure live publication at the current database size. Verify backlog processing, interrupted publication, and repeat rebuilds. |

Completion also requires browser review, appropriate full checks, a verified
running local build, and the branch mirrored to GitHub. Passing the initial
intake tests alone does not complete Phase 8.
