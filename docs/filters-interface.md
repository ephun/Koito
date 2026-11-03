# Filters interface

## Vocabulary and hierarchy

Files contain records. Rules interpret records into plays and the view.
Filters manages those rules.

| Type | Subtypes |
|---|---|
| Matching | Music, Play |
| Inference | Count, Time, Context |
| Attribution | Direct, Heuristic, Aggregate |
| Qualification | Completion, Repetition, Validity |
| Presentation | Naming, Grouping |
| Inclusion | Scope, Evidence level |

Use type headings, then subtype sections, then individual settings.
Add each type when its first usable rule ships.
Show source and file scope beside scoped rules.
Add date scope when its storage and rules are implemented.

## Current layout

| Type | Subtype | Controls |
|---|---|---|
| Matching | Music | Case comparison and missing albums |
| Matching | Play | Time tolerance |
| Inference | Time | Spotify timestamp interpretation and Last.fm timezones |

Each matching section has one On/Off switch.
Its settings become disabled and muted when switched off.
Stored settings remain available when the section is enabled again.

Question marks provide hover and keyboard-focus help.
Help text uses simple sentences with twenty words at most.
Escape dismisses the tooltip.
Tooltips stay within the viewport.

Save changes publishes the rules and view together.
Discard restores the currently loaded settings before saving.
The server checks the active version when saving.
Saved versions and manual decisions remain available for later controls.

Imports and listening history provide record inspection.
Koito's existing search locates music.
Record inspection contains supporting records and contextual matching actions.

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
The underlying legacy methods and endpoints remain implemented.
Their replacement belongs to the catalog redesign.

| User intent | Replacement |
|---|---|
| Identify the same artist or recording | Matching, Music |
| Identify one listen reported by several records | Matching, Play |
| Report album editions together | Presentation, Grouping |
| Choose names for the view | Presentation, Naming |

Use native search to select items for these actions.
Place contextual actions on artist, album, and track pages.
Save each action as a reversible rule.
Keep recording identity and release information distinct.

Audit automatic catalog matching, manual edits, aliases, deletion, and live submissions before adding their replacements.
Include existing legacy data in the migration design.
