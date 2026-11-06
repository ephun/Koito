# Koito fork: development phases

Updated November 5, 2026.

This fork preserves original music data and builds a changeable view of it.
Development and testing happen locally on Windows. Deployment to the server is
a separate, final step.

Phases 1 through 7 are complete. Later phases are proposed
work, not features that already exist. Specific matching thresholds, attribution
heuristics, and display preferences still need examples and decisions as their
phases begin. Old analysis scripts supply examples, not automatically approved
defaults. There are no calendar estimates in this plan.

The development order is **preserved imports first, then the first working
Filters controls, then additional importers and rules together**. Supporting
every file format is not a prerequisite for Filters. Each added importer must
preserve the evidence needed by its rules before those rules affect the view.

## Reading this plan

- Sections 1–3 define the vocabulary, rule types, and shared requirements.
- The phase overview gives the sequence and dependencies.
- Each phase defines its work, acceptance criteria, and unresolved choices.
- The final sections map source formats and upstream issues to phases and list
  the immediate implementation steps.

The local [source survey](../data_dump/SOURCE_SURVEY_2026-11-02.md) supplies the
inventory and historical examples. It contains private source details and is
not part of the repository. This plan contains the coverage needed to work
without publishing those files.

## 1. Vocabulary

| Term | Meaning |
|---|---|
| File | Original uploaded or downloaded bytes, kept unchanged. |
| Record | One entry from a file or incoming submission, with its original fields and source position. |
| Play | One listening event, supported by records or explicitly estimated. |
| Source | Where the data came from: for example Spotify, Last.fm, or a family iTunes library. |
| Filter | One configurable instruction that interprets records or changes the view. |
| View | The history, music metadata, collections, and statistics produced by the filters. |
| Filters | The page where filters are managed, organized by the types below. |

Use **filter** in the interface for each instruction. Existing code and older
implementation notes use **rule** for the same instruction. These are not
separate user concepts. Saved versions remain an implementation responsibility.

There is one user of this fork. There is no account selector on Imports and no
new multi-user workflow to build. Original service identities, device names,
and app names remain source information when the data supplies them. A shared
Spotify history can contain someone else's listening without requiring a second
Koito user.

Records need not describe plays. A record can describe a cumulative count,
playlist entry, owned release, wanted release, concert, or game result.

### Import identity versus matching

- **Import identity:** receiving the same file again does not add another set of
  its records. A renamed file or a separately uploaded archive member must be
  recognized too.
- **Matching:** distinct records can describe the same music or the same play.
  Matching links them; it does not delete them.

Two identical rows at different positions in one file remain two records.
Matching can determine that they represent one play. Two different exports
can retain their own records while contributing to the same plays. Where a
source cannot distinguish identical legitimate repeats from duplicated rows,
retain the ambiguity rather than silently erasing occurrences.

## 2. Rules and their types

| Type | Subtypes | Examples |
|---|---|---|
| Matching | Music matching; play matching | Recognize artist aliases or link Spotify and Last.fm records of one play. |
| Inference | Count; time; context | Estimate plays from iTunes counts, assign a time range, or infer missing context. |
| Attribution | Direct; heuristic; aggregate | Assign observed plays or part of a shared cumulative count to the user. |
| Qualification | Completion; repetition; validity | Decide whether a short skip counts, limit suspected repeat loops, or handle unusable data. |
| Presentation | Naming; grouping | Choose a displayed name or report multiple album editions together. |
| Inclusion | Scope; evidence level | Include concerts, hide podcasts, or include estimated plays. |

Keep the distinctions precise:

- Music matching says two descriptions identify the same artist, recording, or
  release. Grouping lets distinct items appear together in reports.
- Play matching says several records describe one event. Repetition rules act
  on separate events that may nevertheless be excluded from counts.
- Attribution determines whose listening is represented. Inclusion chooses
  which listening appears in a view.
- An estimate remains an estimate even when it counts toward a total.
- Changing a displayed name does not change a source field.

The categories are not a fixed execution order. Each phase must document which
rules depend on others. For example, matching music may precede attribution,
and estimated plays must be checked against observed plays before counting.
Use a deterministic order; do not build a general scripting language as a
prerequisite for the first useful rules.

## 3. Requirements across every phase

### Preserve the evidence

1. Keep original files, complete record fields, repeated row occurrences, and
   links back to their files and positions.
2. Keep source identifiers, timestamp meaning and precision, played duration,
   track duration, and media type separately when available.
3. Record unknown information as unknown. Do not invent a source app, listener,
   exact time, recording identity, or complete listening history.
4. Keep estimates, older processed files, and original exports distinguishable.
5. Make rules, manual corrections, matches, and splits reversible without
   editing the source records.

### Keep the interface small

- Imports has one **Import file** action. Selecting files starts the import.
- Recognize supported content automatically. Unsupported input gets a direct
  error. Do not require an importer-type or account dropdown.
- Show filenames, progress, results, errors, and history as needed. Hide empty
  sections. No explanatory marketing text, implementation panels, repeated file
  labels, or descriptions of what the system promises to preserve.
- Filters shows useful controls and results. Put record-level details behind
  inspection rather than filling the page with explanatory paragraphs.
- If a required fact cannot be detected, preserve it as unknown and expose a
  correction where it belongs. Do not guess silently to avoid asking a question.

### Make changes verifiable

- Each phase has working local behavior and acceptance checks before it is done.
- Compare exact record/play relationships as well as counts. Equal totals can
  hide missing or wrongly combined records.
- Use small, anonymized fixtures in Git and the real dump for local acceptance.
  Do not commit private files, credentials, databases, or development state.
- Keep the existing history usable while replacing individual parts. A failed
  import or rebuild must not damage the previous usable view.

### Storage boundary

The storage model covers the database, original files on disk, their links,
and how a replacement view is published. These are the responsibilities to
preserve as the schema evolves; the table names can follow the existing code.

| What is stored | Required contents | How it changes |
|---|---|---|
| Files on disk | Original upload bytes and extracted member bytes, addressed by hash | Reuse identical bytes; retain original downloads even after a failed import |
| File information and import history | Hash, size, filename, attempt outcome, archive/member links, source | Add attempts and provenance without multiplying records |
| Records | Complete fields, file position, source, reader version | Append new evidence; never overwrite records to clean metadata |
| Rules and manual changes | Type, subtype, scope, settings, enabled state, versions, explicit matches/splits | Save changes with a way to undo them |
| Plays and their supporting records | Every contributing record, chosen fields, timing, estimates and reasons | Rebuild when rules or evidence change |
| View | History, catalog, collections, charts and statistics | Publish a consistent result after a successful rebuild |

Original files currently live beneath
`<config>/sources/sha256/<first two hex>/<sha256>`. The SQLite database stores
their links and extracted records. Both belong in a backup. The repository and
`data_dump` are not the application's permanent storage directories.

Import identity ends at recognizing an existing file and its record occurrences.
Deciding that records describe one play belongs to Matching. Importing must not
discard a record because another service or export appears to cover it.

A reader correction must not create a second copy of the same evidence that
then counts independently. Preserve the original reading, record the correction
or replacement reading, and make the active interpretation explicit. This needs
a defined migration path before changing a reader used by existing imports.

### Publishing and undo

1. Read the preserved records and selected rule versions.
2. Calculate the proposed plays and view, including supporting-record links.
3. Preview changed counts and representative changes when the user edits rules.
4. Verify that the preview still refers to the current files, records, rules,
   and manual decisions before applying it.
5. Publish the complete replacement atomically. On failure, retain the previous
   usable view and the newly stored evidence for retry.
6. Undo by restoring earlier settings or reversing a manual decision, then
   rebuilding. Keep the history of changes.

Rebuilding must be deterministic for the same evidence and rules. Later imports
can change results without rewriting the earlier evidence. Manual corrections
must remain attached to stable records or music identities across rebuilds.

## 4. Phase overview

| Phase | Deliverable | Status / dependency |
|---|---|---|
| 1 | Preserved Spotify imports and a rebuildable view | Complete |
| 2 | Last.fm imports and the first matching rules in Filters | Complete |
| 3 | iTunes counts, inference, and shared-history attribution | Complete |
| 4 | Music identity, album distinctions, bulk cleanup, and genre tags | Complete |
| 5 | Remaining Spotify exports, playlists, and Discogs | Complete |
| 6 | Navidrome, Multi-Scrobbler, and remaining legacy history formats | Complete |
| 7 | Concert and gameplay data | Complete; see [Phase 7](phase-7.md) |
| 8 | Complete live capture and remaining qualification/inclusion controls | After historical import behavior is established |
| 9 | Full-dump migration, restore, and performance verification | After required importers and rules |
| 10 | Local deployment rehearsal, then server deployment | After 9 |

This is the working sequence. Importers that turn out to share most of their
implementation can move earlier without changing the acceptance requirements.

## Phase 1: import and storage foundation

**Status: complete.** Detailed implementation notes: [Phase 1](phase-1.md).

Delivered:

- Separate original-file storage, file hashes, upload history, and immutable
  records with complete JSON payloads and source positions.
- Spotify extended-history JSON and ZIP ingestion through the Imports page.
- Repeat-upload identity, archive/member reuse, and basic overlap handling.
- A minimal, rebuildable view using completed music records and exact matches.
- Inspection of records and download of original files.
- An additive database migration that preserves existing listens and aliases.
- Native Windows build, local server, frontend development server, and tests.

Acceptance evidence includes 100 repeat uploads in tests and a native browser
check using a real file: 8,524 records, 3,383 plays, unchanged after repeated
uploads, ZIP overlap, and rebuilding. Original downloads matched the input bytes.

Limits at Phase 1 completion (Phase 2 resolves the first two):

- The new upload path supports Spotify extended history only. The existing
  startup importers are not equivalent to the new preserved-record path.
- Rules are currently fixed in code; there is no Filters page yet.
- New uploads use the local owner. That is not a judgment that every play in
  a shared-source export belongs to the user.
- Koito's existing display tables can collapse rows at the same track and
  whole-second timestamp. That is a display limitation, not proof of a match.
- Existing live submissions still need the preservation work in Phase 8.

## Phase 2: Last.fm imports and first matching controls

**Outcome:** Spotify and Last.fm histories can overlap without counting an
established play twice, and the matching is visible and reversible in Filters.

**Status: complete.** Implementation details, decisions and acceptance evidence:
[Phase 2](phase-2.md). The local six-file acceptance preserved 477,574 records,
produced 189,831 plays with its explicit UTC test setting, and linked 5,648 plays
across services. Rebuilding and restoring rules reproduced exact contributor
links without changing raw records. These are acceptance-input totals, not a
final lifetime-listening count.

### 2.1 Import Last.fm records

- Support the headerless CSV found in the dump, including its UTF-8 BOM.
- Support the older named-column CSV layout and the JSON/JSONL history forms
  found in earlier projects. Recognize each supported layout from content.
- Preserve original timestamp text. Identify timezone and timestamp precision
  before conversion; missing timezone information must not become an assumed
  UTC time. Handle local-time ambiguity explicitly.
- Preserve missing album names and other sparse metadata without making them
  interchangeable with conflicting, known values.
- Distinguish original Last.fm exports from converted or previously filtered
  files. Overlapping copies do not become independent evidence of extra plays.

### 2.2 Establish saved rules and the Filters page

- Store a rule's type, scope, settings, enabled state, and saved versions.
- Support enable/disable, edit, preview, apply, and undo for the first rules.
- Keep the currently usable view until a complete replacement is ready.
- Show changed counts and representative examples before applying changed
  matching settings. Keep a way to restore the earlier settings and results.
- Keep manual match/split decisions separate from automatic results so a
  rebuild does not erase them.

### 2.3 Match music and plays conservatively

- Begin with reliable identifiers and exact music metadata. Add only the music
  matching needed for this phase; broader naming and grouping come in Phase 4.
- Distinguish the same recording from the album it was played from. Preserve
  each record's release information even when the records match one play.
- Establish what each timestamp means before calculating offsets. Upload time,
  submission time, track start, and track end are not interchangeable.
- Make any timing tolerance a rule setting supported by actual examples from
  the dump, not an unexplained global number.
- Treat multiple possible partners and chains of nearby repeats as ambiguous.
  Do not combine a long sequence of genuine plays through transitive matching.
- Retain qualifying Spotify-only and Last.fm-only plays. Choosing a preferred
  source for displayed fields must not discard unmatched history.
- Let the user inspect contributing records and correct a wrong match or split.

### 2.4 Implementation sequence

| Step | Deliverable | Verification |
|---|---|---|
| 2A | Shared import path for Spotify and the located Last.fm layouts | Repeated, renamed, standalone and ZIP uploads preserve bytes and fields without adding duplicate records |
| 2B | Timestamp interpretation for each layout, including unknown timezone and local-time ambiguity | Epoch, start/end, minute precision and daylight-saving examples remain distinguishable |
| 2C | Saved rules and independent manual match/split history | Settings persist; undo adds a reversible change; rebuilding retains manual decisions |
| 2D | Matching calculation shared by preview and rebuild | Known pairs combine, repeats stay distinct, ambiguous candidates remain inspectable |
| 2E | Atomic replacement of the view with stale-preview detection | Failed replacement leaves the previous view usable; changed evidence requires a fresh preview |
| 2F | Filters page and contributing-record inspection | Edit, preview, apply, undo, match and split work in the browser with concise controls |
| 2G | Local acceptance against overlapping real exports | Record-to-play links, unmatched plays, originals and repeat-import stability are checked together |

Steps may be developed together, but each must pass its own checks. If Koito's
track/whole-second listen key prevents a valid split or collapses genuine plays,
fix that display constraint in this phase. Do not count a collision as a proven
match or invent different timestamps to fit the old key.

**Deferred:** broad alias/regex cleanup, shared-listener attribution, iTunes
count expansion, and the remaining source formats. The first Filters page needs
the rules used here, not placeholder controls for every later type.

**Done when:**

1. Both Last.fm CSV layouts and representative JSON/JSONL files import without
   losing original fields or multiplying records on re-upload.
2. A known Spotify/Last.fm pair produces one play with both records attached.
3. A genuine repeated listen remains a separate play.
4. Ambiguous timing and conflicting music information are not silently merged.
5. Changing and undoing a rule changes the view without changing source files
   or records; manual decisions survive rebuilding.
6. Matching does not hide original album information or unmatched source plays.
7. A preview cannot overwrite newer imports or manual changes; a failed apply
   leaves the previous view intact.

**Decisions recorded:** timezone-less CSV stays unknown until configured;
start/end semantics and precision intervals are preserved; the initial extra
tolerance is zero; ambiguous matches remain separate and inspectable; source
preference changes displayed fields while retaining unmatched plays. See the
Phase 2 notes for the rule order and settings.

## Phase 3: iTunes, inference, and attribution

**Outcome:** family-library counts and shared listening can contribute without
pretending that estimated events are fully observed personal plays.

### 3.1 Import the library as it exists

- Support original iTunes plist XML and the JSON conversion in the dump.
- Preserve library identity, persistent track IDs, media kind, cumulative counts,
  date added, last played, playlists, and complete source metadata.
- Keep cumulative counts as counts. Importing a library is not an instruction
  to generate dated plays.
- Handle missing/zero counts, sentinel dates such as 1904, dates in the wrong
  order, count resets, and repeated library snapshots explicitly.
- Treat later snapshots as updates to evidence about a count, not additional
  copies of the entire reported lifetime count.
- Recognize the older expanded play files as derived data. Their generated
  timestamps and minimum-count assumptions must not replace the library.

The inspected library has 14,020 tracks and 47,748 reported cumulative plays;
the older expansion has 56,210 rows. These are different measurements, not
interchangeable expected totals for an import.

### 3.2 Add inference rules

- Define separate controls for using totals, using known last-played evidence,
  and estimating additional play times. The implemented options are ready for user review.
- Allow undated counts to contribute to suitable totals without inserting
  invented events into calendar charts.
- If time estimates are enabled, keep their supporting interval, uncertainty,
  rule version, and whether the source actually supplied an exact endpoint.
- Make estimates deterministic and check them against observed plays so the
  same listening is not counted through both the aggregate and event history.

### 3.3 Add attribution rules

- Support direct decisions about a source, device, time period, or individual
  record, as well as explicit unknown attribution.
- Support heuristic attribution using available music, device, date, and
  overlapping-history evidence. A track appearing in the user's library is
  evidence to consider, not proof of who listened.
- Support attributing a portion of an aggregate count without claiming to
  identify which exact events were the user's.
- Evaluate earlier source ratios, caps, and thinning scripts against examples.
  Do not silently activate their historical thresholds or random sampling.
- Preserve the total source evidence even when only a portion contributes to
  the user's view.

**Done when:**

1. The same library snapshot can be imported repeatedly without increasing
   either its records or the represented cumulative total.
2. Reported, attributed, estimated, counted, and undated amounts are explainable
   from the source records and enabled rules.
3. Estimates do not exceed the available attributed count or duplicate known
   plays. Disabling inference removes estimates without losing the library.
4. New snapshots, count resets, shared ownership, and invalid dates have tests.
5. The original library's reported counts remain distinguishable from the
   larger number of rows produced by the older expansion script.

**Decisions:** attribution heuristics, acceptable uses of undated counts, inference
options, and the default treatment of uncertain shared listening.

## Phase 4: music matching, album distinctions, and bulk cleanup

**Outcome:** music can be matched, renamed, and grouped without destroying its
original spelling, identifiers, or album association.

**Status: complete.** Track histories across albums are implemented and verified.
The shared evaluator and builder cover Matching, Naming, Grouping, attribution, Count, Time, and imported fields.
Contextual corrections, genres, optional tags, and artist/album matching are implemented.
The complete scope passed closing verification and is mirrored to GitHub.
Implementation and acceptance evidence: [Phase 4](phase-4.md).

### First milestone: track history across albums

Start with the track and album relationship before expanding the filter editor.
This milestone is the beginning of Phase 4. The remaining requirements below
remain outstanding until implemented and verified.

- One track page gathers its listens across album appearances.
- Each play retains its selected album association, including an absent album.
- Supporting records retain every supplied album field and identifier.
- Album histories and totals follow each play's album association.
- Music matching establishes shared music identity. Play matching establishes
  which records describe the same listen.
- Text equivalence can support a Last.fm-style track view. Recording identifiers
  and explicit decisions must retain distinctions between different performances.
- Naming and edition grouping remain separate from identifying one recording.

Audit imported projection and existing live capture together. The current
paths differ in their handling of recording identifiers and absent albums.
Define an additive migration that retains existing track links and listens.
Rebuilding must preserve play identities and contributor links.

Verify the first milestone with real imported records spanning album names
and absent albums. Check track histories, album histories, totals, and raw
records together. Record actual results without inventing listening data.

Then streamline Filters around **IF**, **THEN**, **ELSE IF**, and **ELSE** blocks.
Use one consistent builder, including global behavior with a TRUE condition.
Compose atomic values, comparisons, logical operators, branches, and subtype-specific actions.
Support AND, OR, NOT, NOR, NAND, XOR, and XNOR with explicit nesting.
Use the existing taxonomy to organize each filter.
Source metadata determines which conditions are offered.
The interface uses **filter**, including Add filter and Remove filter.
The [interface design](filters-interface.md#agreed-next-layout) records the builder requirements and migration expectations.
Detailed interaction design follows the working track and album model.

### 4.1 Preserve recording and release distinctions

- Replace the assumption that a recording can belong to only one album.
- Keep the recording, its appearance on a particular release, and the release
  reported for each play distinguishable. Determine the database migration
  from the existing schema rather than choosing tables from names alone.
- A shared MusicBrainz recording ID must not move every play into the first
  album seen. Equal artist/title strings must not prove two performances equal.
- Test singles, compilations, remasters, live recordings, acoustic versions,
  untitled tracks, and repeated titles on different releases.

### 4.2 Add music matching and presentation rules

- Add case-sensitive/case-insensitive matching controls while retaining original
  capitalization and an independent displayed-name choice.
- Add opt-in regex and explicit rules for edition labels such as deluxe and
  remaster, with field/source/artist/release scopes and exceptions.
- Group editions for reporting without deleting their individual identities.
  Do not turn a text-cleaning rule into an unconditional recording merge.
- Add bulk naming corrections, aliases, whitespace/punctuation cleanup, and
  supported artist-name corrections with before/after previews and undo.
- Preserve multi-artist data. Splitting every artist string at a comma is not
  a valid general cleanup rule.
- Make manual corrections win where explicitly intended, and preserve their
  history across future imports and rebuilds.

### 4.3 Add genres and optional metadata display

- Keep source genre tags separately from manual genre assignments.
- Start with artist genres and chart filtering; preserve richer track/release
  tags when available without requiring a new visualization first.
- Resolve source conflicts through rules rather than overwriting values.
- Add optional, concise source/metadata tags to listening rows. Arbitrary fields
  remain inspectable even when they have no special display component.

**Done when:**

1. Plays from different albums retain the correct album even when recording
   identifiers match.
2. A grouping or case rule can be enabled and undone without changing originals.
3. Regex changes show affected items before application and preserve exceptions.
4. Genre filtering works with source and manual tags.
5. Database edits used for cleanup are represented as reversible rules or
   corrections; the user does not need destructive merges for routine cleanup.

This phase addresses [#359](https://github.com/gabehf/Koito/issues/359),
[#360](https://github.com/gabehf/Koito/issues/360),
[#34](https://github.com/gabehf/Koito/issues/34),
[#225](https://github.com/gabehf/Koito/issues/225), and the display portion of
[#361](https://github.com/gabehf/Koito/issues/361). Exact matching behavior needed
in Phase 2 must already avoid discarding these distinctions.

## Phase 5: remaining Spotify exports, playlists, and Discogs

**Status:** complete. See [implementation notes](phase-5.md) and the
[closing audit](phase-5-audit.md) for verified behavior and source coverage.

**Outcome:** the view can use streaming history, saved music, and physical
collections without treating all of them as listening events.

### 5.1 Spotify standard history and account exports

- Add the standard history JSON layouts for music, podcasts, and audiobooks.
- Preserve their lower timestamp precision and missing identifiers. Do not
  pretend minute-level records have the fidelity of the extended export.
- Match overlapping standard and extended history using the rules from Phase 2.
- Recognize supported parts of an account ZIP while preserving the entire
  original file. Account/technical data that is not listening remains context.

### 5.2 Playlists and saved music

- Add Spotify playlist/library JSON and the playlist CSV exports in the dump.
- Preserve playlist identity, ordering, repeated entries, added dates, track IDs,
  ISRCs, and other supplied fields.
- Keep snapshots separate from changes in membership. Re-imports must not make
  a playlist grow or manufacture extra likes/saves.
- Use membership for music matching, attribution evidence, and inclusion where
  appropriate. It does not produce a play by default.

### 5.3 Discogs

- Support collection, wantlist, contributions, inventory, orders, and order-items
  CSVs, including the collection ZIP and valid header-only files.
- Preserve release IDs and physical-copy distinctions. Several owned copies
  of the same release are not necessarily duplicates.
- Preserve condition, notes, format, label, catalog number, dates, and unknown
  years. A release year of zero must not appear as an actual year.
- Handle the observed wantlist's extra timestamp column without discarding it
  or silently assigning an unverified meaning.
- Make owned/wanted/saved states available in the view. Buying, adding, or owning
  an album is not a dated listen.

**Done when:** repeated and overlapping exports leave memberships and quantities
stable; legitimate repeated playlist entries and physical copies survive; the
view can include ownership/preferences without increasing listening totals;
standard/extended Spotify overlap does not double-count plays.

## Phase 6: Navidrome, Multi-Scrobbler, and legacy history formats

**Outcome:** local playback and existing forwarding history can be preserved
without counting delivery records as new listening.

### 6.1 Navidrome

- Accept a consistent SQLite snapshot, including required WAL contents when
  supplied as a bundle. Do not read a changing live database as a fixed export.
- Import catalog metadata, tags, player information, scrobbles, and cumulative
  annotations as their distinct record types.
- Verify timestamp meanings, especially submission time versus listening time.
- Reconcile scrobbles and cumulative counts; do not add both wholesale.
- Use local catalog metadata for matching/genres without silently replacing
  conflicting source values.

### 6.2 Multi-Scrobbler

- Support the located SQLite schema, source inputs, app/component references,
  parent/child relationships, queue state, and submission results.
- Link an incoming play and its Koito/Last.fm destination copies to the same
  listening evidence. A retry, success log, or failed delivery is not another play.
- Add version-aware reading of the older Flatted/Typeson cache and relevant log
  records. Preserve unsupported versions rather than guessing their structure.
- Keep original source, forwarding app, device, endpoint, and arbitrary extra
  fields distinct when present. Unknown route details stay unknown.
- Identify empty database backups and irrelevant logs directly; do not infer
  successful playback or coverage from their filenames.

### 6.3 Existing importer formats

- Move supported ListenBrainz, Maloja, and Koito exports onto the preserved-file
  and record path. Retain compatibility with the working legacy importers until
  equivalent behavior is verified.
- Cover the earlier normalized JSONL/SQLite forms needed for migration, with
  original-versus-derived status recorded.

**Done when:** one Spotify play forwarded to two destinations still represents
one play; repeated database snapshots/retries do not inflate totals; Navidrome
counts do not duplicate event records; unknown fields survive every supported
path. Configured sources with no records are not reported as captured history.

## Phase 7: concerts and gameplay

**Status: complete.** Implementation, verified formats, actual-data results,
and the closing requirements audit are documented in [Phase 7](phase-7.md).

**Outcome:** concert attendance and game performances can contribute through
explicit rules while remaining distinguishable from recorded audio playback.

### 7.1 Setlist.fm

- Import preserved API/download data when available and retain existing Koito
  concert-derived records as legacy evidence.
- Keep performance identity, date, venue, artist, sets, song order, covers,
  revisions, and attendance evidence separately where supplied.
- Recover original performance references where possible. No standalone source
  export was located in the dump, so completeness remains unproven until checked.
- Do not treat the old 20:00 UTC starts and 5m15s spacing as observed song times.
  Handle date-only performances and estimated times explicitly.
- Re-importing a corrected setlist updates evidence about the same performance;
  it does not add another concert. Preserve genuine repeated songs and encores.
- Distinguish the performing artist from the original artist of a cover.

### 7.2 YARG/YARGfm and Clone Hero

- Import YARG game records, player scores, player identity, and relevant song
  metadata. Check the meaning and timezone of stored game dates.
- Treat several player scores for one game as separate score records, not
  automatically several listens. Handle replays, bots, speed, and completion.
- Reconcile game records with corresponding YARGfm scrobbles.
- Inspect and document Clone Hero binary versions before claiming support.
  Archive unrecognized files and give a direct unsupported-format result.

**Done when:** a repeated performance import adds no duplicate event; inferred
concert times remain marked; a multiplayer game does not multiply listening;
gameplay/concert inclusion can be changed without losing source records.

## Phase 8: live capture and remaining filter controls

**Outcome:** ongoing submissions receive the same preservation and rule handling
as historical imports.

### 8.1 Preserve incoming data

- Preserve the current server history and Multi-Scrobbler database, queues, and
  configuration before changing routing. Listening since the last export is
  part of the migration, including the days spent developing this fork.
- Backfill that interval from current retained sources with an overlapping
  boundary, preserving original timestamps and matching repeated evidence.
- Add the development fork as a separate destination while existing destinations
  continue receiving listening. Validate connectivity before enabling delivery.
- Keep complete accepted request bytes and record fields before acknowledging
  completed-listen submissions. Include unknown metadata fields.
- Retain source/device/app identifiers and original field meanings. Do not
  flatten every input into an unexplained Multi-Scrobbler label.
- Keep now-playing updates separate from completed plays; handle stop, expiry,
  ordering, retries, and restart recovery.
- Use native request/event identifiers where available. Identical content alone
  is not always proof of a network retry rather than a legitimate repeated play.
- A failure to update the view must not discard an accepted record.
- Test the actual Multi-Scrobbler-to-Koito path. Data excluded upstream before
  being sent cannot be recovered by Koito; record that coverage honestly.

### 8.2 Finish qualification and inclusion

- Add controls for played time, completion evidence, repetition, media type,
  source, date ranges, estimated plays, and uncertain attribution as needed.
- Keep actual played duration distinct from recording duration. A skip, missing
  duration, and confirmed completion are different facts.
- Make repeat-loop limits operate after relevant attribution/matching rather
  than confusing shared listening or duplicate records with excessive playback.
- Use the same rules for history, search, statistics, charts, and exports, or
  make an explicitly chosen difference visible. Avoid unrelated hidden filters.

**Done when:** native live submissions retain arbitrary fields, retries do not
multiply plays, now-playing never becomes a completed play by itself, outages
recover without loss, and changing rules treats live and imported records
consistently. This completes the capture portion of
[#361](https://github.com/gabehf/Koito/issues/361).

## Phase 9: full-dump migration, backup, and performance

**Outcome:** the complete collection of supported source data can be imported
locally, explained, backed up, and rebuilt without relying on the old database
as an unquestioned source of truth.

- Finish bulk intake for the packaging actually needed by the dump: ZIP,
  TAR.GZ, and selected folder batches. Use the same record readers as individual
  uploads, preserve container/member links, and bound nesting and expanded size.
  Bring a shared container reader forward if an earlier source requires it.
  A folder batch is a selection of files, not permission to import every file
  from an environment or server backup as listening data.
- Build a file inventory for the full dump: original exports, repeated copies,
  archive members, older processed files, database snapshots, and supporting
  documents. Identify by content rather than assuming paths imply uniqueness.
- Reuse hashes for duplicate files without losing the history of where copies
  were found. Keep nested-backup lineage.
- Import the existing Koito data and aliases without losing edits or mistaking
  legacy/generated listens for newly observed events.
- Compare the old analysis outputs against the new view as diagnostics. Do not
  add filtered_plays, top_tracks, inferred iTunes rows, and original histories
  together as if they were independent listening.
- Archive PDFs/scripts/settings as supporting files when deliberately included.
  Do not execute uploaded code or require parsers for every dependency file in
  a server backup. Empty dvinyl directories and empty Floppy music tables do not
  require new source integrations.
- Support a full backup containing source files, records, rules and their
  versions, and manual changes. A database-only backup is insufficient.
- Restore into a new local directory and compare file hashes, raw payloads,
  record-to-play relationships, metadata, and relevant view results.
- Test interrupted imports/rebuilds and recovery at realistic data sizes. Record
  import, rebuild, query time, memory, and disk use before setting targets.
- Improve batching, indexing, and progress reporting where measured behavior
  requires it. Keep long operations inspectable without verbose UI prose.

**Done when:** every intended source file is accounted for, unexplained differences
are recorded, repeats do not inflate the view, and a fresh restore reproduces
the results from the preserved files/records/rules rather than cached totals.

## Phase 10: deployment rehearsal and server rollout

**Outcome:** the locally verified fork runs on the server with a tested way back.

- Use `fm.ephun.net` as the preferred address for the fork, replacing the
  current Koito at `stats.ephun.net`. The preferred Multi-Scrobbler interface
  address is `fm.ephun.net/scrobbler`, pending verification on the installed
  version. Both applications remain separate services behind the reverse proxy.
- Test Multi-Scrobbler's subpath support with
  `BASE_URL=https://fm.ephun.net/scrobbler`, following its
  [deployment documentation](https://docs.multi-scrobbler.app/installation/).
  Check dashboard assets, API routes, streaming connections, and authentication
  callbacks. Update registered callback addresses where needed.
- Inventory current submission URLs before changing proxy routes. Preserve
  delivery from existing players and scrobblers throughout the address change.
  Keep browser redirects separate from API submission routing.
- Build the fork's image locally and test the same migrations and import/view
  behavior in its Linux environment. Pin the tested build rather than deploying
  upstream's latest image by mistake.
- Before the first Docker build, add build exclusions for `data_dump`, `.local`,
  credentials, databases, and dependency/build directories. There is currently
  no root `.dockerignore`; `COPY . .` must not package local source data.
- Prepare the deployment configuration, persistent database and source-file
  volumes, backup procedure, and restore/rollback instructions.
- Rehearse migration using a copy of the server data. Preserve original files,
  metadata, existing edits, and any unresolved evidence.
- Verify live routing and existing destinations during rehearsal. Historical
  rebuilding must not re-send old listens to external services.
- Review the concrete tested image/configuration and intended routing changes
  with the user before changing the production server.
- After rollout, check health, live now-playing/completed plays, source capture,
  record counts, view results, and backup/restore behavior.

**Done when:** the server uses the tested fork, persistent storage and backups
include original files, live and historical checks pass, and rollback has been
rehearsed. Native Windows remains the regular development environment.

## 5. Source coverage checklist

"Planned" means support must be implemented and verified; recognizing a filename
or retaining a ZIP alone is not equivalent to reading its relevant records.

| Source / material found | Required forms | Phase |
|---|---|---|
| Spotify extended history | Individual JSON, multi-file ZIP, renamed supported files | 1 complete |
| Last.fm history | Headerless/named CSV; source JSON; earlier JSONL conversions | 2 |
| Family iTunes | Original plist XML, converted JSON, count snapshots, identified legacy expansions | 3 |
| Music identities and metadata | Original strings/IDs, recording/release associations, aliases, genre tags | 2 foundation; 4 full controls |
| Spotify standard/account history | Music/podcast/audiobook JSON, relevant account ZIP contents | 5 |
| Spotify playlists/library | Playlist/library JSON and playlist CSV ZIPs | 5 |
| Discogs | Collection ZIP/CSV, wantlist, contributions, inventory, orders, order-items | 5 |
| Navidrome | Consistent SQLite data, catalog, annotations, scrobbles, supplied WAL bundle | 6 |
| Multi-Scrobbler | SQLite inputs/events, versioned cache forms, useful log records, source metadata | 6 history; 8 live |
| ListenBrainz/Maloja/Koito exports | Existing supported history formats migrated to preserved records | 6 |
| Setlist.fm | Original API/download records where recoverable; identified legacy concert records | 7 |
| YARG/YARGfm | Score SQLite, song metadata, matching game/scrobble evidence | 7 |
| Clone Hero | Documented score/cache binary versions | 7, after format verification |
| Earlier projects | Needed normalized CSV/JSON/JSONL/SQLite; mappings, reviews, summaries, rule settings | 3/4/6/9, according to record meaning |
| Backup containers and folder batches | ZIP, TAR.GZ, selected folder files; shared readers, nested-member links and size limits | ZIP foundation in 1; remaining bulk intake in 9 or earlier when needed |
| Last.fm app PDF, scripts, settings, unknown files | Supporting-file preservation or direct unsupported result; no invented plays | 9 |
| Live submissions | Full accepted bodies and arbitrary fields, separate now-playing handling | 8 |

## 6. Linked issue coverage

These links describe requested outcomes, not proof that upstream or this fork
has implemented them. Summaries were checked while writing this plan.

| Issue | Requested outcome | Work in this fork |
|---|---|---|
| [#361: source metadata](https://github.com/gabehf/Koito/issues/361) | Preserve arbitrary input fields and optionally display source/field tags | Phase 1 starts file preservation; 4 adds optional display; 6 and 8 cover remaining history/live paths. |
| [#360: edition grouping](https://github.com/gabehf/Koito/issues/360) | Opt-in bulk grouping of deluxe/remastered editions, including incoming metadata | Phase 4 regex/grouping rules with originals and exceptions preserved. |
| [#359: case handling](https://github.com/gabehf/Koito/issues/359) | Configurable case-insensitive music grouping/matching | Phase 4 controls with independent original and displayed names. |
| [#225: genres](https://github.com/gabehf/Koito/issues/225) | Genre assignments usable in charts | Phase 4 manual/source tags and chart filtering; later source imports supply more tags. |
| [#34: album distinctions](https://github.com/gabehf/Koito/issues/34) | Preserve which album a play came from even when recording identity matches | Phase 2 preserves record-level release information; 4 revises the catalog/view relationships. |

## 7. Checks before closing each phase

1. Check its requirements against the actual working code and local behavior.
2. Run focused tests for changed behavior, the applicable Go suite, client type
   checks, and a production client build when frontend changes require it.
3. Exercise representative real files locally; retain byte/record comparisons,
   not just a successful status or plausible total.
4. Verify repeated input, failure/retry, rebuilding, and original preservation.
5. Check new controls in the browser, including small screens. Keep the single
   upload action and avoid reintroducing rejected explanatory UI.
6. Record remaining limitations and decisions; do not describe later-phase work
   as complete or silently choose personal counting preferences.
7. Commit and push to the working branch. Production deployment is not part of
   closing an earlier phase.

## 8. Immediate next work

Phase 4 is complete. See [the Phase 4 notes](phase-4.md) and
[requirement audit](phase-4-audit.md) for implementation and verification.

Phase 5 is complete. See [its notes](phase-5.md) and [closing audit](phase-5-audit.md).
Phase 6 is complete. See [its notes](phase-6.md) and [closing audit](phase-6-audit.md).
Phase 7 covers concerts, gameplay, and their reconciliation with the preserved history.

Field-specific source preference also needs concrete examples before controls
return to Filters. Keep original music identities and source album associations
available throughout the catalog redesign.
