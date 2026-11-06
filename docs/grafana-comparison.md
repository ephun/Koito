# Koito or Grafana

Status: architectural recommendation, based on source and documentation review on November 6, 2026.
One application is required. Choosing Grafana means replacing Koito as the application.
A separate Koito service feeding Grafana is outside this comparison.

## Recommendation

Continue with Koito as the single application. Use an established visualization library for editable charts.
Keep Grafana as the alternative application foundation, rather than an additional dashboard service.

This recommendation follows from the complete music application requirements, not a unique ability to draw charts.
Grafana already provides a mature dashboard editor. Building equivalent general-purpose tooling would duplicate substantial work.
Koito already supplies the music application, and this fork supplies preserved Files, Records, Filters, and the View.
Replacing it with Grafana would require moving that behavior into a custom music application plugin.
Existing code may be reusable, but integration, public access, storage, and migration would require verification.

The proposed native editor is substantial development work too. A rendering library does not supply its user interface,
query planner, saved definitions, permissions, or cache invalidation automatically.

## What each application provides

The Koito column refers to this fork unless explicitly marked as planned.
These are differences in built-in behavior. Grafana can be extended to implement the missing behavior.

| Requirement | Koito | Grafana as the replacement application |
|---|---|---|
| Live listening and now playing | Existing music intake and pages. | Requires custom intake, authentication, persistence, and playback presentation. |
| Historical imports | Existing parsers, preserved originals, records, and reimport handling. | Requires a custom import backend and interface. |
| Non-destructive interpretation | Existing matching, inference, attribution, qualification, presentation, and inclusion Filters. | Requires porting the engine, saved configuration, editor, and publication behavior. |
| Music identities and evidence | Existing artist, album, track, Play, and Record relationships. | Requires a music data model and interfaces for those relationships. |
| Editable ordinary charts | Existing charts mostly use fixed components. A general editor is planned. | Existing panel editing, field configuration, transformations, and dashboard layout. |
| Music-aware query editor | Planned reuse of typed fields and expression controls. | Requires a music data source and query editor, or acceptance of database-specific queries. |
| Music detail pages | Existing pages, search, artwork, and listening history. | Custom application pages or dashboards could provide these. |
| Last.fm Stats calculations | Complete requested inventory remains planned. | Some fit queries and transforms. Domain-specific calculations still require implementation. |
| Rich Rewind and designed exports | Planned reusable definitions, layouts, and rendering. | Custom panels or application pages would be needed for the full requested experience. |
| Koito visual style | Existing native theme and components. | Recreating the style across Grafana pages and panels requires integration work. |
| Public viewing and admin editing | Existing application access model, with new chart publication still to build. | Existing dashboard access features, with important differences between public-sharing modes. |
| Fast results after interpretation changes | Requires analytical summaries, revision-aware caches, and faster publication. | Requires the same domain-side work. Grafana cannot infer which results a matching change invalidates. |

[Grafana panels](https://grafana.com/docs/grafana/latest/visualizations/panels-visualizations/)
combine queries and visualizations. Its
[transformations](https://grafana.com/docs/grafana/latest/visualizations/panels-visualizations/query-transform-data/transform-data/)
already cover operations such as joining, renaming, and calculations.
[Data links](https://grafana.com/docs/grafana/latest/visualizations/panels-visualizations/configure-data-links/)
can carry an entity and time range to another page. Music drilldowns are therefore possible in Grafana.

## The viable single-application Grafana design

Grafana supports [application plugins](https://grafana.com/developers/plugin-tools/tutorials/build-an-app-plugin)
containing custom pages, panels, and data sources. A
[Go backend component](https://grafana.com/developers/plugin-tools/how-to-guides/app-plugins/add-backend-component)
can execute server-side work, and
[resource handlers](https://grafana.com/developers/plugin-tools/how-to-guides/app-plugins/add-resource-handler)
can expose custom HTTP operations.

That makes a self-contained music application within Grafana technically plausible.
It would own Files, Records, storage, imports, scrobbling, Filters, View publication, and analytical queries.
Grafana would own the application shell and ordinary dashboard editing.
Koito would cease to run as a separate application.

This is a feasibility inference from the extension APIs, not a completed migration design.
Scrobbler protocol routes, token handling, durable background work, uploads, backups, and recovery need a working proof.
Grafana runs plugin backends as managed subprocesses, so lifecycle behavior also needs validation.
That is still one user-facing application, with Grafana's normal plugin architecture.

## Three representative visualization cases

| Case | Grafana route | Native Koito route | Assessment |
|---|---|---|---|
| Monthly plays by artist, editable as lines, bars, or areas | Music query results feed standard panels. Grafana supplies much of the editing. | Backend query plus declarative rendering, with controls and persistence to implement. | Grafana has the strongest existing tooling here. |
| LastWave-style streamgraph | A custom panel or chart plugin can consume grouped music results. | Vega-Lite supplies centered stacking. Exact contour labels may need a custom renderer. | Neither route automatically reproduces every LastWave detail. |
| Rewind with discoveries, streaks, artwork, and music drilldowns | Custom calculations plus panels or application pages. | The same calculations, existing music pages, and reusable recap layouts. | Native integration is a practical advantage. Neither route already supplies the requested recap. |

[Business Charts](https://grafana.com/grafana/plugins/volkovlabs-echarts-panel/)
integrates Apache ECharts and supports custom JavaScript configuration, visual editing, and additional chart types.
Grafana is therefore not limited to its standard panel catalog.
Exact LastWave output has not been validated with this plugin.
[Vega-Lite's centered stacking](https://vega.github.io/vega-lite/docs/stack.html)
likewise establishes a streamgraph foundation, not an already-delivered music editor.

These are implementation comparisons, not benchmark results. No Grafana instance or native chart prototype was run for this report.
No latency or development-time claims are measurements.

## Public access and performance

Grafana's [externally shared dashboards](https://grafana.com/docs/grafana/latest/visualizations/dashboards/share-dashboards-panels/shared-dashboards/)
offer public read-only access, but currently exclude variables and queries containing variables.
They also require compatible backend data sources.
This matters for configurable public music exploration.

[Anonymous Viewer access](https://grafana.com/docs/grafana/latest/setup-grafana/configure-access/configure-authentication/anonymous-auth/)
is another route. It should not be confused with sharing only saved dashboard queries.
Grafana's [data source permissions documentation](https://grafana.com/docs/grafana/latest/administration/data-source-management/)
states that Viewers can normally issue other queries against available data sources.
A replacement should expose the intended public View through its backend, with private Records and Files separately authorized.

Grafana's built-in data source query caching is an Enterprise/Cloud feature.
An OSS music plugin could implement caching itself.
Either application needs summaries and cache keys tied to the published View revision.
Changing track matching can alter old periods, so expiration alone does not establish correctness.
Current Koito uses Recharts and request-result caching. The broader analytical layer is still planned.
Neither choice alone fixes View rebuild latency.

## Next development step

Keep Koito and prove a bounded native editor before expanding the proposal into a general dashboard platform.
Use actual imported listening for the ordinary chart, streamgraph, and recap cases above.
Require editable grouping, measures, conditions, chart form, theme inheritance, and music links.
Verify that different graphical presentations reuse the same calculated result.
Measure cold and cached requests separately, alongside View publication delay.

Use the existing typed expressions where meanings agree. Grouping, windows, and database execution remain additional work.
Keep the entire requested reference inventory as the acceptance scope, implemented through reusable operators and editable presets.
Do not reduce customization to a list of fixed chart components.

If the native prototype requires rebuilding broad dashboard infrastructure before these examples work,
reconsider replacing the application with Grafana plus the music plugin described above.
Do not maintain both applications as the proposed product.
Navigation and phase renumbering should follow the foundation decision and working prototype.
This recommendation does not authorize deployment or changes to personal Filters.
