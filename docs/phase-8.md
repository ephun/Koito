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

A continuous local capture saves complete Last.fm history pages from November 1
and periodic Multi-Scrobbler API pages. These originals include overlap with the
baseline. The selected backfill added 1,590 records to the working database.
Repeating those imports added zero records. The working view rebuild remains
under verification. Record counts are not unique-play counts.

The installed Multi-Scrobbler version is 0.17.2. Its ListenBrainz conversion
forwards service, player, recording identifiers, and recording duration. It omits
native event identifiers, device identifiers, and actual listened duration.
Retaining its API pages preserves richer source evidence alongside submissions.
The fork cannot reconstruct fields omitted from an incoming request.

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
undated counts. Export provenance and presentation parity still need review.

Entry-page typography and layout use stock Koito classes again. Playlist
membership is hidden from entry pages while imported memberships remain stored.
The user's saved personal filter configuration has not been replaced.

Backend tests and client type checking/build pass at this checkpoint. The local
receiver still needs the latest build, browser verification, and working-data
recovery/performance checks before completion. The GitHub branch is a development
checkpoint, not a deployment approval.

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
