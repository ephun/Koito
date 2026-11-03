import { useEffect, useState, type ReactNode } from "react";
import { Navigate, useSearchParams } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useAppContext } from "~/providers/AppProvider";
import { Option, Switch, filterButton as button, filterInput as input, filterCard as card } from "~/components/FilterControls";
import { AddScopedRule, ScopedRule, libraryRule, subtypeName } from "~/components/ScopedFilterRules";
import { filterRequest, FilterRequestError, type FilterState, type Rule, type RuleConfig, type RuleSettings, type RuleVersion } from "~/utils/filterRules";

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

  function edit(id: string, update: Partial<RuleSettings> | { enabled: boolean }) {
    setDraft(previous => previous && ({ rules: previous.rules.map(rule => {
      if (rule.id !== id) return rule;
      return "enabled" in update ? { ...rule, enabled: update.enabled } : { ...rule, settings: { ...rule.settings, ...update } };
    }) }));
    setMessage("");
  }

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

  async function save() {
    if (!saved || !draft || saving) return;
    if (draft.rules.some(rule => rule.enabled && Object.entries(rule.scope).some(([key, value]) => key !== "service" && key !== "source_id" && (value === "" || value === 0)))) {
      setError("Complete or remove empty scope conditions.");
      return;
    }
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

  const music = draft?.rules.find(rule => rule.type === "matching" && rule.subtype === "music");
  const plays = draft?.rules.find(rule => rule.type === "matching" && rule.subtype === "play");
  const time = draft?.rules.find(rule => rule.type === "inference" && rule.subtype === "time" && rule.scope.service !== "itunes" && !rule.scope.file_hash && !rule.scope.source_id);
  const librarySources = saved?.sources.filter(source => source.service === "itunes" && source.records > 0) || [];
  const countRules = visibleLibraryRules("count");
  const libraryTimes = visibleLibraryRules("time");
  function visibleLibraryRules(subtype: "count" | "time") {
    const rules = draft?.rules.filter(rule => rule.type === "inference" && rule.subtype === subtype && rule.scope.service === "itunes") || [];
    return rules.length ? rules : librarySources.map(source => libraryRule(source, subtype));
  }
  const countEnabled = (timeRule: Rule) => countRules.some(rule => rule.enabled &&
    (["source_id", "file_hash", "library_id", "track_id", "record_id", "artist"] as const).every(key => !rule.scope[key] || !timeRule.scope[key] || rule.scope[key] === timeRule.scope[key]));
  const dirty = !!saved && !!draft && JSON.stringify(saved.current.config) !== JSON.stringify(draft);

  function fileTimezone(file: FilterState["time_files"][number], zone: string) {
    if (!draft || !time) return;
    const existing = draft.rules.find(rule => rule.type === "inference" && rule.subtype === "time" && rule.scope.service !== "itunes" && rule.scope.file_hash === file.hash && (!rule.scope.source_id || rule.scope.source_id === file.source_id));
    if (zone === "inherit") {
      if (existing) upsert({ ...existing, enabled: false });
      return;
    }
    upsert({ ...(existing || { id: `time-${file.source_id}-${file.hash}`, type: "inference", subtype: "time", scope: { service: "lastfm", source_id: file.source_id, file_hash: file.hash }, settings: { ...time.settings } }), enabled: true, settings: { ...(existing?.settings || time.settings), timezone: zone } });
  }

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
    {loading && <p role="status">Loading rules...</p>}
    {message && <p role="status" className="text-sm text-(--color-fg-secondary)">{message}</p>}

    {music && plays && <section aria-labelledby="matching-title" className="space-y-3">
      <h2 id="matching-title">Matching</h2>
      <div className={card}>
        <RuleGroup title="Music matching" label="Music matching" rule={music} busy={saving} onToggle={enabled => edit(music.id, { enabled })}>
          <Option label="Case" id="music-case" help="Choose how capitalization affects music matching.">
            <select id="music-case" className={input} disabled={saving || !music.enabled} value={music.settings.case_sensitive ? "exact" : "ignore"} onChange={event => edit(music.id, { case_sensitive: event.target.value === "exact" })}>
              <option value="exact">Match exact case</option>
              <option value="ignore">Ignore case</option>
            </select>
          </Option>
          <Option label="Missing albums" id="missing-albums" help="Allow matches when one record has an empty album.">
            <Switch id="missing-albums" label="Match missing albums" checked={music.settings.match_missing_album} disabled={saving || !music.enabled} onChange={match_missing_album => edit(music.id, { match_missing_album })} />
          </Option>
        </RuleGroup>
        <RuleGroup title="Play matching" label="Play matching" rule={plays} busy={saving} onToggle={enabled => edit(plays.id, { enabled })}>
          <Option label="Time tolerance" id="play-tolerance" help="Allow extra seconds between matching timestamps.">
            <span className="flex items-center gap-2"><input id="play-tolerance" aria-label="Time tolerance in seconds" className={`${input} w-24`} type="number" min="0" max="3600" step="1" disabled={saving || !plays.enabled} value={plays.settings.tolerance_seconds} onChange={event => edit(plays.id, { tolerance_seconds: Number(event.target.value) })} /><span className="text-sm text-(--color-fg-secondary)">seconds</span></span>
          </Option>
        </RuleGroup>
      </div>
    </section>}

    {plays && <section aria-labelledby="inference-title" className="space-y-3">
      <h2 id="inference-title">Inference</h2>
      <div className={card}>
        {!!librarySources.length && saved && <div>
          <h3 className="px-4 pt-4 text-lg font-medium normal-case tracking-normal sm:px-5 sm:pt-5">Count</h3>
          {countRules.map(rule => <ScopedRule key={rule.id} rule={rule} state={saved} busy={saving} onChange={upsert} onRemove={draft?.rules.some(existing => existing.id === rule.id) ? () => remove(rule.id) : undefined} />)}
          <AddScopedRule state={saved} type="inference" subtype="count" busy={saving} onAdd={upsert} />
        </div>}
        <section className="space-y-4 p-4 sm:p-5" aria-label="Time inference">
          <h3 className="text-lg font-medium normal-case tracking-normal">Time</h3>
          <Option label="Spotify timestamps" id="spotify-time" help="Estimate starts using the end time and played duration.">
            <select id="spotify-time" className={input} disabled={saving} value={plays.settings.spotify_time} onChange={event => edit(plays.id, { spotify_time: event.target.value })}>
              <option value="estimated_start">Estimated start</option><option value="end">Reported end</option>
            </select>
          </Option>
          {!!saved?.time_files.length && time && <div className="space-y-3 border-t border-(--color-bg-tertiary) pt-4">
            <Option label="Last.fm timezone" id="lastfm-timezone" help="Choose the timezone used by Last.fm export dates.">
              <TimezoneSelect id="lastfm-timezone" label="Last.fm timezone" disabled={saving} value={time.enabled ? time.settings.timezone : ""} onChange={timezone => upsert({ ...time, enabled: true, settings: { ...time.settings, timezone } })} />
            </Option>
            <details>
              <summary className="cursor-pointer text-sm text-(--color-fg-secondary)">Timezones by file</summary>
              <div className="mt-3 divide-y divide-(--color-bg-tertiary)">{saved.time_files.map(file => {
                const rule = draft?.rules.find(rule => rule.type === "inference" && rule.subtype === "time" && rule.scope.service !== "itunes" && rule.scope.file_hash === file.hash && (!rule.scope.source_id || rule.scope.source_id === file.source_id));
                return <div key={`${file.source_id}-${file.hash}`} className="space-y-3 py-4">
                  <p className="break-all text-sm font-medium">{file.filename}</p>
                  <Option label="Timezone" help="Use a different timezone when this file needs one.">
                    <TimezoneSelect label={`Timezone for ${file.filename}`} disabled={saving} inherit value={rule?.enabled ? rule.settings.timezone : "inherit"} onChange={zone => fileTimezone(file, zone)} />
                  </Option>
                </div>;
              })}</div>
            </details>
          </div>}
          {!!librarySources.length && saved && <div className="divide-y divide-(--color-bg-tertiary) border-t border-(--color-bg-tertiary)">
            {libraryTimes.map(rule => <ScopedRule key={rule.id} rule={rule} state={saved} busy={saving} blocked={!countEnabled(rule)} onChange={upsert} onRemove={draft?.rules.some(existing => existing.id === rule.id) ? () => remove(rule.id) : undefined} />)}
            <AddScopedRule state={saved} type="inference" subtype="time" busy={saving} onAdd={upsert} />
          </div>}
        </section>
      </div>
    </section>}

    {saved?.sources.some(source => source.records > 0) && <section aria-labelledby="attribution-title" className="space-y-3">
      <h2 id="attribution-title">Attribution</h2>
      {["direct", "heuristic", "aggregate"].map(subtype => {
        const rules = draft?.rules.filter(rule => rule.type === "attribution" && rule.subtype === subtype) || [];
        return !!rules.length && <div key={subtype} className="space-y-3">
          <h3 className="text-lg font-medium normal-case tracking-normal">{subtypeName(subtype)}</h3>
          <div className={card}>{rules.map(rule => <ScopedRule key={rule.id} rule={rule} state={saved} busy={saving} onChange={upsert} onRemove={() => remove(rule.id)} />)}</div>
        </div>;
      })}
      <div className={card}><AddScopedRule state={saved} type="attribution" busy={saving} onAdd={upsert} /></div>
    </section>}
  </main>;
}

function RuleGroup({ title, label, rule, busy, onToggle, children }: { title: string; label: string; rule: Rule; busy: boolean; onToggle: (enabled: boolean) => void; children: ReactNode }) {
  return <section className="space-y-4 p-4 sm:p-5" aria-label={label}>
    <div className="flex items-center justify-between gap-4"><h3 className="text-lg font-medium normal-case tracking-normal">{title}</h3><Switch label={label} checked={rule.enabled} disabled={busy} onChange={onToggle} /></div>
    <div className={`${rule.enabled ? "" : "opacity-50"} space-y-4`}>{children}</div>
  </section>;
}

function errorMessage(error: unknown) { return error instanceof Error ? error.message : "Could not save rules."; }

function TimezoneSelect({ id, label, value, disabled, inherit = false, onChange }: { id?: string; label: string; value: string; disabled: boolean; inherit?: boolean; onChange: (value: string) => void }) {
  const available = (Intl as typeof Intl & { supportedValuesOf?: (key: string) => string[] }).supportedValuesOf?.("timeZone") || ["America/Denver", "America/New_York", "America/Los_Angeles"];
  const zones = [...new Set(["UTC", ...available, ...(value && value !== "inherit" ? [value] : [])])];
  return <select id={id} aria-label={label} className={`${input} w-full sm:w-64`} value={value} disabled={disabled} onChange={event => onChange(event.target.value)}>
    {inherit && <option value="inherit">Use Last.fm timezone</option>}
    <option value="">Unknown</option>
    {zones.map(zone => <option key={zone} value={zone}>{zone.replaceAll("_", " ")}</option>)}
  </select>;
}
