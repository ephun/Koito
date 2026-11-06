# Phase 9 pause checkpoint

The user paused Phase 9 on November 6, 2026 and requested a Grafana-based
replacement design in `../GrafanaMusicVis` beside this repository.

Phases 1 through 8 and the phase-plan commit `95d1534` remain preserved.
The following preliminary changes were written before the pause:

- `FilterBlockEditor.tsx` exports the existing ExpressionEditor for future reuse.
- `031_widgets.sql` proposes Widget storage, query caching, and revision tracking.

These changes are unfinished. The migration has not been applied to the working
database. No Phase 9 acceptance tests or completed Widget interface exist.
They are checkpointed for preservation, rather than presented as a working feature.

No receiver restart, source migration, personal Filter change, or production
configuration change was performed for this checkpoint. Existing capture continues
through the Phase 8 runtime.

The replacement design must cover the complete Koito fork behavior and planned
visualization additions. Grafana's visual style is acceptable for the new design.
Koito's development plan remains a reference, rather than the active objective.
