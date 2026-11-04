# Filters interface

## Vocabulary and hierarchy

The taxonomy in [the development plan](development-plan.md#2-rules-and-their-types) is authoritative.
Use its existing types and subtypes when adding controls.

Files contain records. Filters interpret records into plays and the view.
Filters is also the page name. Each configurable instruction is a filter.
Existing code calls these instructions rules. That distinction stays internal.

| Type | Subtypes |
|---|---|
| Matching | Music matching, play matching |
| Inference | Count, Time, Context |
| Attribution | Direct, Heuristic, Aggregate |
| Qualification | Completion, Repetition, Validity |
| Presentation | Naming, Grouping |
| Inclusion | Scope, Evidence level |

Use type headings, then subtype sections, then individual settings.
Add each type when its first usable rule ships.
Show source and file scope beside scoped rules.
Date scope is available for supported attribution and time rules.

## Agreed next layout

Streamline conditional filters into a table with When and Then columns.
Keep the existing types and subtypes as its hierarchy.
When selects records or record pairs. Then defines the action.
Available conditions follow the metadata supplied by each source.
Simple global settings can remain compact controls.
Use Add filter and Remove filter in the interface.

Implement the track and album relationship before expanding this editor.
A track page should gather listens across album appearances.
Each play retains its album association and contributing records.
Album histories and totals follow the album associated with each play.
Music matching and play matching remain separate decisions.
The sections below describe the currently implemented interface.

## Current layout

| Type | Subtype | Controls |
|---|---|---|
| Matching | Music matching | Case comparison and missing albums |
| Matching | Play matching | Time tolerance |
| Inference | Count | Library totals, latest counts, and explicit reset interpretation |
| Inference | Time | Spotify timestamps, Last.fm timezones, and library timing |
| Attribution | Direct | Personal, other, or unknown listening |
| Attribution | Heuristic | Explicit personal evidence and minimum matching plays |
| Attribution | Aggregate | A selected share of cumulative counts |

Each matching section has one On/Off switch.
Its settings become disabled and muted when switched off.
Stored settings remain available when the section is enabled again.

Question marks provide hover and keyboard-focus help.
Help text uses simple sentences with twenty words at most.
Escape dismisses the tooltip.
Tooltips stay within the viewport.
Source headings keep their help icons on the same row.

Last.fm timezone uses a dropdown.
The current export uses UTC, corroborated by matching Spotify timestamps.
Unknown remains available for unidentified exports.
The timezone setting has no On/Off control.
Timezones by file appears with multiple relevant files or enabled exceptions.
Its optional exceptions default to using the Last.fm timezone.
iTunes count help explicitly refers to iTunes library export records.
Its time controls use last-played dates and estimated play dates as their visible language.

Save changes publishes the rules and view together.
Discard restores the currently loaded settings before saving.
The server checks the active version when saving.
Saved versions and manual decisions remain available for later controls.

Imports and listening history provide record inspection.
Koito's existing search locates music.
Record inspection contains supporting records and contextual matching actions.
Library inspection also shows count contributions and attribution.
Library timing controls depend on Count rules.
Scope conditions are added individually inside Scope.
Context labels summarize selected artist, device, track, and date conditions.
The source preference design remains separate from attribution controls.

## Source preference

Source preference needs field-specific rules and scopes.
One overall source ranking combines several different decisions.
Names, album information, play time, and listener attribution need separate treatment.

Use Spotify, Last.fm, and Navidrome examples to design these rules.
Include forwarded copies, converted files, and sparse metadata in those examples.
Distinguish playback sources from forwarding apps.
Treat ownership and playlist membership as supporting records.

Naming choices belong under Presentation.
Play time choices belong under Matching or Inference, depending on the evidence.
Attribution determines the listener represented by the records.

Existing source preference settings remain stored during this interface change.
The source preference control will return after its scope is defined.

## Legacy Merge

Koito's current Merge operations modify catalog relationships directly.
Artist merges delete the original artist after moving its links.
Album merges move tracks between releases.
Track merges redirect listens.
Cleanup removes entries that become orphaned.

The routine editing toolbar now omits Merge.
The legacy endpoints, handlers, database methods, and unused client controls are removed.
Reversible replacements belong to the catalog redesign.

| User intent | Replacement |
|---|---|
| Identify the same artist or recording | Matching, music matching |
| Identify one listen reported by several records | Matching, play matching |
| Report album editions together | Presentation, Grouping |
| Choose names for the view | Presentation, Naming |

Use native search to select items for these actions.
Place contextual actions on artist, album, and track pages.
Save each action as a reversible rule.
Keep recording identity and release information distinct.

Audit automatic catalog matching, manual edits, aliases, deletion, and live submissions before adding their replacements.
Include existing legacy data in the migration design.

## Heading and condition refinements

Type headings include their purpose in hover and keyboard help.
Attribution subheadings sit inside their cards, matching the other filter types.
The iTunes date controls are labeled iTunes play dates under Time.
Count, Time, and Context remain the inference subtypes.
Conditions is the interface label for a rule's scope.
New conditions omit internal record and persistent track identifiers.
Existing identifier conditions remain editable.
Device conditions currently apply to Spotify's supplied platform metadata.
Future conditions should follow each source adapter's available metadata.
Filters loads source identities and record availability independently from Imports contribution summaries.

iTunes inference controls omit the redundant source selector.
Source selection remains available where several source types are meaningful.
