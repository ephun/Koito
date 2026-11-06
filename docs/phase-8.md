# Phase 8: live capture and remaining filter controls

Status: in progress. The acceptance requirements remain those in
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

## Acceptance audit

| Requirement | Evidence still required before completion |
|---|---|
| Preserve complete accepted bytes and unknown fields | Unit and HTTP checks pass. Verify deployed receiver with real submissions. |
| Retry and restart recovery | Unit checks pass for explicit retry keys, publication failure, and restart replay. Verify actual sender retry behavior. |
| Separate now-playing from completed plays | Unit checks cover timestamped updates, expiry, stop, ordering, completion, and restart. Verify rendered behavior. |
| Preserve source, device, app, and native identifiers | Initial ListenBrainz fields retained and interpreted. Verify Multi-Scrobbler's actual outgoing fields and distinct event behavior. |
| Preserve current history, database, queues, and configuration | Existing historical backups and current API pages located. Confirm current configuration and queue backup coverage before editing the server. |
| Backfill since the last export with overlap | Reader tested on original saved pages. Import into the working database, compare coverage with current destinations, and investigate gaps. |
| Add the fork as another destination | Connectivity and authentication verified. Configure and observe real delivery after receiver verification. |
| Remaining qualification and inclusion controls | Implement and test repetition, validity, source/date/media conditions, evidence level, and uncertain attribution where missing. |
| Repeat limits after matching and attribution | Verify distinct plays remain distinct and shared listening does not trigger false loop limits. |
| Consistent history, search, statistics, charts, and exports | Verify every affected read path against the same applied filters. |
| Existing native behavior | Finish integration checks, preserve manual submissions, and verify any configured relay behavior. |
| Working-data performance and recovery | Measure live publication at the current database size. Verify backlog processing, interrupted publication, and repeat rebuilds. |

Completion also requires browser review, appropriate full checks, a verified
running local build, and the branch mirrored to GitHub. Passing the initial
intake tests alone does not complete Phase 8.
