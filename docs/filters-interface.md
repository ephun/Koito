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

Use one filter manager with a consistent condition-and-action builder.
The manager summarizes filters in a table with IF and THEN columns.
Keep the existing types and subtypes as its hierarchy.
IF evaluates a condition. THEN specifies its actions.
ELSE IF adds another conditional branch. ELSE specifies the remaining branch.
Available conditions follow the metadata supplied by each source.
Global behavior uses the same builder with TRUE as its IF condition.
Existing behavior can supply editable starting filters.
Each filter has one enabled state beside its conditions and actions.
Parameter controls belong inside the action they configure.
Use Add filter and Remove filter in the interface.

Implement the track and album relationship before expanding this editor.
A track page should gather listens across album appearances.
Each play retains its album association and contributing records.
Album histories and totals follow the album associated with each play.
Music matching and play matching remain separate decisions.
The builder requirements below describe planned behavior.
The Current layout section describes the implemented interface.

### Atomic building blocks

Use familiar IF, THEN, ELSE IF, ELSE, AND, OR, and NOT labels.
The editor assembles nested blocks with selectable fields and typed inputs.
Each block performs one operation and can receive another compatible block as input.
Indentation and visible nesting define grouping.
The table summarizes the block structure.

| Block | Purpose | Examples |
|---|---|---|
| Value | Supply one value | Original album, displayed album, a constant, another record's title |
| Value operation | Produce a value from inputs | Trim text, replace text, add numbers, subtract timestamps |
| Comparison | Test values | Equals, greater than, contains, is missing |
| Logic | Combine or negate conditions | AND, OR, NOT, NOR |
| Branch | Select actions through conditions | IF, THEN, ELSE IF, ELSE |
| Action | Apply one interpretation or view change | Set a displayed field, set listener, link music identities |

These are builder elements within the existing filter taxonomy.
They do not introduce additional filter types.
Existing behaviors can supply templates assembled from these elements.
Templates remain editable through the same builder.

### Logical operators

AND, OR, and NOT form the basic logical operations.
Offer the following named combinations directly in the operator selector.
Normalize their saved expressions into the basic operations while retaining their chosen display labels.

| Operator | Meaning for known Boolean inputs | Equivalent expression |
|---|---|---|
| AND | Both conditions are true | `A AND B` |
| OR | At least one condition is true | `A OR B` |
| NOT | Reverse a condition | `NOT A` |
| NOR | Neither condition is true | `NOT (A OR B)` |
| NAND | The conditions are not both true | `NOT (A AND B)` |
| XOR | Exactly one condition is true | `(A OR B) AND NOT (A AND B)` |
| XNOR | Both conditions have the same truth value | `NOT ((A OR B) AND NOT (A AND B))` |

AND and OR can accept multiple nested conditions.
NOT accepts one condition. XOR and XNOR compare two conditions.
NAND and NOR negate their corresponding AND and OR groups.
Make group boundaries explicit whenever operators are combined.
Use operator names in the interface rather than substituting All and Any labels.

### Comparisons and values

- Start with readable field, comparison, and value rows.
- Support `=`, `!=`, `<`, `<=`, `>`, and `>=` for compatible value types.
- Offer CONTAINS, STARTS WITH, ENDS WITH, IN, and MATCHES REGEX for their applicable types.
- Offer IS MISSING, IS PRESENT, and IS EMPTY as distinct tests.
- Distinguish absent fields, null values, empty text, empty lists, and numeric zero.
- Support explicit null checks when source data supplies null values.
- Offer TRUE and FALSE constants for unconditional behavior.
- Offer arithmetic `+`, `-`, `*`, and `/` with typed inputs and explicit units.
- Offer text operations such as TRIM, NORMALIZE CASE, REPLACE, and REGEX REPLACE.
- Assemble case-insensitive equality by normalizing both compared values.
- Offer date and duration operations with explicit timezone interpretation.
- Build a range from lower and upper comparisons, retaining inclusive or exclusive bounds.
- Offer COUNT, SUM, MIN, and MAX over an explicitly selected collection and field.
- Identify whether an aggregate counts records, plays, or library evidence.
- Offer field-to-field comparisons for music and play matching.
- Identify which record supplies each field when comparing a pair.
- Offer conditions on filenames, sources, metadata, and available identifiers through readable labels.
- Distinguish original source fields from derived fields when both are available.
- Preserve the complete source metadata even before dedicated controls exist for a field.
- Conditions depend on available source fields and their data types.

An absent field is distinct from an empty string or zero.
Ordinary comparisons against absent fields remain unknown.
Only a true condition selects a record or pair.
Missing and present checks explicitly select those cases.
Negating a comparison against absent data must retain that unknown result.
This matters for attribution when source metadata is incomplete.

### IF, ELSE IF, and ELSE

```text
IF <condition>
THEN
    <action>
ELSE IF <condition>
THEN
    <action>
ELSE
    <action>
END IF
```

ELSE IF and ELSE are optional.
Evaluate branches in order and execute the first branch whose condition is true.
Advance to the next branch only when the previous condition is false.
ELSE runs after every preceding condition evaluates false.
If a required condition remains unknown, leave this filter unresolved and expose the reason during inspection.
Use IS MISSING or IS PRESENT to explicitly handle incomplete evidence.
An omitted ELSE applies no action from this filter when all conditions are false.

Each branch may contain ordered actions or another IF block.
Keep ordering within a filter distinct from precedence between filters.
Select branches using the values available at that point in the declared processing stage.
Avoid implicitly repeating a filter after its own actions change a field.

For example, a Presentation, Naming filter can choose a displayed album:

```text
IF (original album IS MISSING) OR (original album = "")
THEN
    SET displayed album TO "Unknown Album"
ELSE
    SET displayed album TO original album
END IF
```

The displayed album is a derived field. Original album values remain preserved.

### Actions

Use the agreed type and subtype to offer meaningful actions.
The builder structure stays consistent across types.
Action controls reflect the operation's actual parameters.
Build actions from single-purpose operations with explicit targets and inputs.
Allow an action input to contain a compatible value expression.
Setting a displayed field, assigning a listener, and linking identities are separate actions.
An action's target determines which other actions can share its branch.
Stateful inference operations retain explicit inputs, semantics, and inspectable contributions.

| Type | Example condition | Example action |
|---|---|---|
| Matching, music matching | Artist and title agree, one album is absent | Allow a music match |
| Matching, play matching | Music matches and compatible times overlap | Link records describing one listen |
| Inference, count | Records come from an iTunes library export | Use the cumulative play count |
| Attribution, direct | Source and supplied device match selected values | Attribute listening to me |
| Presentation, naming | Album name matches a selected pattern | Apply a replacement to the displayed name |
| Presentation, grouping | Album editions match selected conditions | Report those editions together |

These examples specify the design. They do not claim every action currently exists.
Album compatibility alone establishes neither recording identity nor a shared listening event.
Music matching, play matching, and presentation grouping retain their distinct meanings.
Count actions operate on cumulative evidence rather than manufacturing observed plays.

### Manager and editor

Show enabled state, filter type, IF summary, and THEN summary in each row.
Summarize ELSE IF and ELSE branches when present.
Open the same editor when adding or changing a filter.
Support named filters, duplication, removal, and visible execution order where relevant.
Use selectors for fields, operators, and actions.
Use source-provided choices where useful, while allowing suitable typed values.
Use native music search when selecting specific artists, tracks, or albums.
Keep nested IF blocks, logical groups, and regular expressions available within the editor.
Use brief hover help beside unfamiliar fields and operations.
Show validation beside the condition or action that needs correction.
Keep the normal manager focused on configured filters.
Show examples beside pattern edits when checking their effect requires them.
Keep affected records inspectable without adding a permanent preview dashboard.

### Execution and compatibility

The builder needs typed value expressions, conditions, branches, and actions beneath the interface.
Fixed scope fields and subtype-specific switches cannot represent the agreed expression model alone.
The server must validate fields, operators, value types, branches, action parameters, and dependencies.
Define operator behavior for absent inputs, invalid arithmetic, and explicit conversions.
Retain unknown results through compound logic rather than converting them into false.
For example, FALSE AND unknown is false. TRUE OR unknown is true. NOT unknown remains unknown.
Use the same evaluator for inspection and publishing.
Define available derived fields at each processing stage.
Detect circular dependencies before saving.
Evaluate record matching, play matching, and cumulative evidence in their appropriate contexts.

Order matters for sequential naming replacements.
Conflicting attribution actions require an explicit resolution rather than an accidental display-order winner.
Document action-specific precedence and expose order only where it changes behavior.
Preserve existing manual matching and splitting decisions across saves and rebuilds.
Retain the current version check and atomic publication behavior.

Translate existing settings into equivalent starting filters when migrating the saved configuration.
Verify the same play identities, contributor links, timestamps, attribution, and aggregate contributions before changing behavior.
Preserve disabled settings and file-specific timezone exceptions.
Unsupported saved conditions need an explicit migration outcome and must remain recoverable.
Compare full relationships alongside totals using actual imported data.

### Design references

- [Scratch IF/ELSE](https://scratch.mit.edu/help/studio/tips/blocks/if-else/) supplies the familiar branching vocabulary.
- Scratch's [AND](https://scratch.mit.edu/help/studio/tips/blocks/and/), [OR](https://scratch.mit.edu/help/studio/tips/blocks/or/), and [NOT](https://scratch.mit.edu/help/studio/tips/blocks/not/) demonstrate composable logical blocks.
- [IFTTT Applets](https://ifttt.com/docs/applets) demonstrates IF/THEN composition and configurable action inputs.

Our ELSE branches, additional operators, and music-specific actions are this fork's design requirements.

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
