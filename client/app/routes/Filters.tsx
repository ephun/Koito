import { useEffect, useState } from "react";
import { Navigate, useSearchParams } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import FieldHelp from "~/components/FieldHelp";
import ProgramFilterManager from "~/components/ProgramFilterManager";
import { useAppContext } from "~/providers/AppProvider";
import { filterButton as button } from "~/components/FilterControls";
import { filterRequest, FilterRequestError, type FilterState, type Rule, type RuleConfig, type RuleVersion } from "~/utils/filterRules";

export default function Filters() {
  const { user } = useAppContext();
  const queries = useQueryClient();
  const [params] = useSearchParams();
  const recordID = params.get("record");
  const [saved, setSaved] = useState<FilterState>();
  const [draft, setDraft] = useState<RuleConfig>();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [stale, setStale] = useState(false);
  const [message, setMessage] = useState("");

  async function load() {
    setLoading(true);
    setError("");
    setStale(false);
    try {
      const state = await filterRequest<FilterState>();
      state.current.config.rules = state.current.config.rules.map(rule => state.blocks?.legacy?.[rule.id] || rule);
      state.current.config.rules.push(...(state.blocks?.additions || []).filter(rule => !state.current.config.rules.some(existing => existing.id === rule.id)));
      state.sources ||= [];
      state.files ||= [];
      state.libraries ||= [];
      setSaved(state);
      setDraft(structuredClone(state.current.config));
    } catch (error) {
      setError(errorMessage(error));
    } finally { setLoading(false); }
  }

  useEffect(() => {
    if (user?.role === "admin" && !recordID) void load();
  }, [user?.role, recordID]);

  function upsert(rule: Rule) {
    setDraft(previous => previous && ({ rules: previous.rules.some(existing => existing.id === rule.id)
      ? previous.rules.map(existing => existing.id === rule.id ? rule : existing)
      : [...previous.rules, rule] }));
    setMessage("");
  }
  function remove(id: string) {
    setDraft(previous => previous && ({ rules: previous.rules.filter(rule => rule.id !== id) }));
    setMessage("");
  }

  function move(id: string, direction: -1 | 1) {
    setDraft(previous => {
      if (!previous) return previous;
      const rules = [...previous.rules];
      const index = rules.findIndex(rule => rule.id === id);
      const rule = rules[index];
      if (!rule) return previous;
      let other = index + direction;
      while (other >= 0 && other < rules.length && (rules[other].type !== rule.type || rules[other].subtype !== rule.subtype)) other += direction;
      if (other < 0 || other >= rules.length) return previous;
      [rules[index], rules[other]] = [rules[other], rules[index]];
      return { rules };
    });
    setMessage("");
  }

  async function save() {
    if (!saved || !draft || saving) return;
    setSaving(true);
    setError("");
    setMessage("");
    setStale(false);
    try {
      const version = await filterRequest<RuleVersion>("/save", { base_version: saved.current.id, config: draft });
      setSaved({ ...saved, current: version });
      setDraft(structuredClone(version.config));
      await queries.invalidateQueries();
      setMessage("Saved.");
    } catch (error) {
      setError(errorMessage(error));
      setStale(error instanceof FilterRequestError && error.status === 409);
    } finally { setSaving(false); }
  }

  const dirty = !!saved && !!draft && JSON.stringify(saved.current.config) !== JSON.stringify(draft);

  if (recordID) return <Navigate replace to={`/imports?record=${encodeURIComponent(recordID)}`} />;
  if (user?.role !== "admin") return <main className="p-6"><h1>Filters</h1><p>Administrator login required.</p></main>;

  return <main className="w-full max-w-4xl space-y-8 p-4 sm:p-8">
    <title>Filters | Koito</title>
    <header className="flex flex-wrap items-center justify-between gap-4">
      <h1>Filters</h1>
      <div className="flex items-center gap-3">
        {dirty && <button type="button" className="px-2 py-2 text-(--color-fg-secondary) hover:underline disabled:opacity-50" disabled={saving} onClick={() => { setDraft(structuredClone(saved!.current.config)); setMessage(""); setError(""); }}>Discard</button>}
        <button type="button" className={button} disabled={!dirty || saving || loading} onClick={save}>{saving ? "Saving..." : "Save changes"}</button>
      </div>
    </header>

    {error && <div role="alert" className="rounded border border-(--color-error) p-4">
      <p>{error}</p>
      {(stale || !saved) && <button type="button" className="mt-2 underline" onClick={load}>Reload</button>}
    </div>}
    {loading && <p role="status">Loading filters...</p>}
    {message && <p role="status" className="text-sm text-(--color-fg-secondary)">{message}</p>}

    {saved && draft && <>
      <section aria-labelledby="matching-title" className="space-y-3">
        <div className="flex items-center gap-2"><h2 id="matching-title">Matching</h2><FieldHelp label="Matching">Identify the same music, or records describing the same listen.</FieldHelp></div>
        {["music", "play"].map(subtype => <ProgramFilterManager key={subtype} state={saved} config={draft} busy={saving} type="matching" subtype={subtype} embedded onChange={upsert} onRemove={remove} />)}
      </section>
      <section aria-labelledby="inference-title" className="space-y-3">
        <div className="flex items-center gap-2"><h2 id="inference-title">Inference</h2><FieldHelp label="Inference">Derive missing quantities, dates, or context from available evidence.</FieldHelp></div>
        {saved.sources.some(source => source.service === "itunes" && source.records > 0) && <ProgramFilterManager state={saved} config={draft} busy={saving} type="inference" subtype="count" embedded onChange={upsert} onRemove={remove} />}
        <ProgramFilterManager state={saved} config={draft} busy={saving} type="inference" subtype="time" embedded onChange={upsert} onRemove={remove} />
      </section>
      {saved.sources.some(source => source.records > 0) && <section aria-labelledby="attribution-title" className="space-y-3">
        <div className="flex items-center gap-2"><h2 id="attribution-title">Attribution</h2><FieldHelp label="Attribution">Determine which listening belongs to you.</FieldHelp></div>
        {["direct", "heuristic", "aggregate"].map(subtype => <ProgramFilterManager key={subtype} state={saved} config={draft} busy={saving} type="attribution" subtype={subtype} embedded onChange={upsert} onRemove={remove} />)}
      </section>}
    </>}
    {saved && draft && <section aria-labelledby="presentation-title" className="space-y-3">
      <div className="flex items-center gap-2"><h2 id="presentation-title">Presentation</h2><FieldHelp label="Presentation">Choose how music names and groups appear in the view.</FieldHelp></div>
      {["naming", "grouping"].map(subtype => <ProgramFilterManager key={subtype} state={saved} config={draft} busy={saving} subtype={subtype} embedded onChange={upsert} onRemove={remove} onMove={move} />)}
    </section>}
  </main>;
}


function errorMessage(error: unknown) { return error instanceof Error ? error.message : "Could not save filters."; }
