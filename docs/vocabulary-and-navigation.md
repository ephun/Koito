# Vocabulary and navigation proposal

Status: proposal for discussion. Existing data vocabulary and filter taxonomy are retained.
This consolidates the earlier visualization proposal. No application navigation changes have been made.

## Governing distinctions

- A data concept needs its own meaning and lifecycle.
- An editor operation needs consistent behavior across the subsystems using it.
- A page is a place to work or browse. It need not introduce a new data concept.
- A sidebar, menu, tab, panel, and dialog are interface components.
- Equivalent concepts should use the same name and implementation.

The main architectural vocabulary is File, Record, Source, Play, Filter, View, Widget, and Dashboard.
The first six are existing vocabulary. Widget and Dashboard are proposed additions.

## Data vocabulary

| Term | Definition | Why it remains distinct |
|---|---|---|
| Source | The origin of data, including supplied service identity, library identity, or other provenance. | One origin can supply many files and records. |
| File | Preserved original input bytes from an upload, download, archive member, or incoming request. | The complete original exists independently of successful extraction. |
| Record | An original entry with its fields, position, file, and source information. | Entries remain inspectable even when they contribute no listening. |
| Play | One interpreted listening event, supported by records or explicitly estimated. | Several records can describe one event. A record can also describe something other than listening. |
| Filter | One saved instruction that interprets evidence or changes the View. | It changes shared data interpretation rather than the configuration of one graphic. |
| View | The interpreted history, music metadata, collections, and related results produced through Filters. | It is the data state consumed by pages and widgets. It is not a page or dashboard. |

Undated count contributions remain quantities supported by records. They do not become invented dated events.
Keep their quantities and evidence available to suitable totals through the View.
An estimate remains identified as an estimate. Missing information remains distinct from zero or false.

Import identity prevents repeated ingestion of the same file from multiplying its records.
Play matching determines whether distinct records describe the same listening event.
Use those specific names when discussing the two mechanisms. Unqualified deduplication is ambiguous.

## Music and library vocabulary

| Term | Definition and relationship |
|---|---|
| Artist | A credited music identity, interpreted through Matching and Presentation filters. |
| Track | The music identity used for shared listening history. Matching determines which versions belong together. |
| Album | The interpreted album identity or grouping supplied by the View. |
| Album appearance | A track's relationship to a particular album, retaining version-specific metadata. This is a relationship rather than another music page type. |
| Playlist | A named, ordered collection of membership records. Membership does not itself imply listening. |
| Collection | Recorded ownership or catalog membership. Ownership does not itself imply listening. |
| Tag | A descriptive value attached to music or listening. |
| Genre | A music-classification kind of tag. Existing genre controls can retain that familiar label. |

Aliases, recording identifiers, release dates, credits, artwork, and durations are metadata fields and relationships.
They need precise meanings but do not each require another top-level page.
The original source's names and identifiers remain in Records when the View uses different identities or names.

## Shared editor vocabulary

| Term | Definition | Use |
|---|---|---|
| Field | A named, typed value available in the current context. | Record title, played duration, artist identity, or a calculated result. |
| Operator | A defined operation with typed inputs and output. | AND, NOR, equality, addition, SUM, date operations. |
| Expression | Fields, values, and operators composed to produce a value. | Conditions and calculations in either editor. |
| Condition | An expression producing a Boolean decision. | Decides whether a filter acts or whether data enters a widget calculation. |
| Action | An assignment or operation performed by a Filter. | Changes an allowed interpretation target within that filter's stage. |
| Calculation | An expression or analytical operation producing a derived result. | Group totals, averages, ranks, intervals, and graphical values. |
| Selection | The data currently chosen by controls or conditions. | Date range, selected artist, or a chart region. Its persistence is explicit. |
| Parameter | A named input exposed by a definition. | A date range, session gap, grouping interval, or threshold. |

These describe program parts rather than additional top-level saved libraries.
The existing expression engine, value types, validation, and compatible editor controls should be shared.
Fields and allowed outputs vary by execution context.

Query describes the internal read plan for selections, grouping, and calculations.
It initially belongs to a widget's Data and Calculations configuration. A separate Query page or saved Query entity is unnecessary.

Setting is an ordinary word for a configurable property. It does not name a new execution primitive.
Rule remains a legacy synonym for Filter in older code, not a second user-facing concept.

## Filter vocabulary

The established taxonomy remains unchanged:

| Type | Subtypes |
|---|---|
| Matching | Music; play |
| Inference | Count; time; context |
| Attribution | Direct; heuristic; aggregate |
| Qualification | Completion; repetition; validity |
| Presentation | Naming; grouping |
| Inclusion | Scope; evidence |

The Filters page manages these instructions with the shared conditional and expression controls.
Widget conditions use compatible expressions without creating persistent Filters.
Presentation filters govern music names and grouping. Widget Appearance governs graphical styling.

## Visualization vocabulary

| Term | Definition | Boundary |
|---|---|---|
| Widget | One configurable presentation containing its data request, calculations, display definition, and exposed parameters. | The saved concept for graphs, tables, lists, statistics, and generated graphics. |
| Dashboard | A saved arrangement of widget references, sizes, and shared parameter values. | Provides a page composition rather than another calculation engine. |
| Preset | A starting configuration for an existing kind of object. | A preset creates or configures a Filter, Widget, or Dashboard. It introduces no second execution system. |

Chart is a graph-like display of a Widget. It is not a separate saved object beneath a Widget.
Layout means a Dashboard's arrangement. It is not a separate product concept.
Statistics means calculated content. It does not require a separate public page.
Visualization names the subsystem and its output, rather than another object alongside Widget.

A Dashboard stores references to Widgets plus placement and parameter bindings.
Editing a shared Widget updates its uses. Duplicating it creates an independent Widget.
Dashboard-specific date selections can vary through exposed parameters without copying the entire definition.

Home is the Dashboard designated as the site's landing page.
Rewind is a recap Dashboard preset with period comparisons and optional story presentation.
Report describes a Dashboard or export presentation. It does not require another saved object type.

## Site structure and placement

| Destination | Purpose | Placement | Access |
|---|---|---|---|
| Home | The designated public Dashboard, including the listening feed and selected visualizations. | Sidebar home icon. | Visitors read published content. Admin edits. |
| Other Dashboards | Distinct published arrangements such as a recap or historical exploration. | Dashboard selector or links within content. | Visitors read published content. Admin edits. |
| Expanded Widget | More space, exposed parameters, underlying results, and export controls. | Open from the Widget. Keep a linkable address. | Published widgets are readable publicly. Editing is admin-only. |
| Artist, album, track pages | Contextual music details and scoped visualizations. | Search, rankings, history, and other music links. | Public View data, with admin controls when signed in. |
| Listening history | The expanded listening feed with navigation through time. | Open from Last Played. | Public View data. |
| Search | Find music within the existing native search system. | Sidebar search button. | Public search over appropriate published data. |
| Settings | Hub for preferences, account controls, and administrative tools. | Bottom sidebar gear. | Contents depend on role. |
| Imports | Ingestion, original files, sources, import attempts, and record inspection. | Admin Settings link and contextual links. | Admin. |
| Filters | Persistent data interpretation and View configuration. | Admin Settings link and contextual links. | Admin. |
| Widgets | Library of saved Widget definitions and editable presets. | Admin Settings link and Add/Edit controls. | Admin authoring. Published outputs appear elsewhere. |

A public Widget or Dashboard can have its own address without becoming a permanent sidebar destination.
The public site should expose selected work, rather than mirror the entire authoring library.
The old standalone Statistics destination is withdrawn from the recommendation.

### Sidebar

Start with Search, Home, and the Settings gear.
Keep the existing visual style, sizing, spacing, and icon language.
Add a permanent shortcut only when it serves a frequent, distinct destination.
A separate Rewind shortcut remains an option if usage demonstrates its value.
It still opens the same recap Dashboard rather than another implementation.

### Settings hub

Retain the name Settings instead of introducing overlapping Settings, Manage, and Administration destinations.
Organize its contents into sections rather than creating additional domain concepts:

- Appearance: site defaults and any explicitly supported personal appearance preferences.
- Account: sign-in details and API keys.
- Data: Imports, Filters, exports, and source/connection tools.
- Dashboards and widgets: saved definitions, arrangement, and publishing controls.

Imports, Filters, and complex editors open their own full pages.
They should not be squeezed into the current Settings dialog.
The gear can open a compact navigation menu or a full hub as its contents grow.
Empty space is not a reason to add more settings or retain unrelated sidebar buttons.

Dashboard editing happens in place through an admin Edit dashboard action.
Widget editing uses one shared editor reached from the Widget or its library entry.
Record inspection remains part of Imports, including links from timestamps.
The Multi-Scrobbler interface remains a separate administrative application linked through connection tools.

## Public and admin behavior

Visitors receive published Dashboards and Widgets backed by the intended View data.
The administrator chooses which parameters visitors can change, such as a date range or artist selection.
Visitor interaction changes their current selection, not the saved Widget or shared Filters.

Only the administrator edits definitions, publishes content, changes Filters, imports data, or inspects original Records and Files.
Draft definitions remain private. Publication permissions apply to data requests as well as visible controls.
Each saved definition can expose useful interaction while preserving the administrator's design.

## Operations and supporting terms

- Import is an ingestion operation. Its attempts have outcomes and history. A repeated attempt can reuse the same File and Records.
- Submission is an incoming live request using the preservation and ingestion path.
- Now Playing is playback state supported by Records. It is separate from a completed Play.
- Export creates an output file from selected data or a Widget/Dashboard rendering.
- Save stores a definition or configuration. Publish makes an intended result available to readers.
- Version identifies a revision of a particular object or published data state. Always qualify which object is versioned.
- Evidence describes supporting Records and the basis of interpreted values. It remains inspectable through those relationships.
- Page is a navigable interface location. Panel, dialog, menu, tab, and sidebar describe interface components.
- Admin and visitor describe access roles. They do not introduce multiple listening accounts or attribution identities.

## Consolidations from earlier proposals

| Earlier terms | Consolidated decision |
|---|---|
| Chart definition plus Widget definition | Widget, containing data/calculation/display configuration. |
| Home plus an overlapping Statistics page | Home Dashboard and contextual expanded Widgets. |
| Separate Rewind application | Recap Dashboard preset, including story mode. |
| Saved Layout as another object | Arrangement stored in the Dashboard. |
| Template and Preset | Preset. |
| Rule and Filter | Filter. |
| Manage, Administration, and Settings hubs | Settings, organized by task and role. |
| Query library plus chart library | Widget library initially, with its Data and Calculations editors. |

This vocabulary defines responsibilities, not implementation milestones or approved screen redesigns.
The proposal should be agreed before changing navigation or building the visualization system.
