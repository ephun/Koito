import { useEffect, useState, type ReactNode } from "react";
import { Navigate, useSearchParams } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useAppContext } from "~/providers/AppProvider";
import FieldHelp from "~/components/FieldHelp";
import { filterRequest, FilterRequestError, type FilterState, type Rule, type RuleConfig, type RuleSettings, type RuleVersion } from "~/utils/filterRules";

const button = "rounded border border-(--color-bg-tertiary) px-4 py-2 hover:bg-(--color-bg-tertiary) disabled:opacity-50 disabled:cursor-default";
const input = "min-w-0 max-w-full rounded border border-(--color-bg-tertiary) bg-(--color-bg) px-3 py-2 disabled:cursor-default";
const card = "rounded-lg border border-(--color-bg-tertiary) bg-(--color-bg-secondary) divide-y divide-(--color-bg-tertiary)";

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

  const music = draft?.rules.find(rule => rule.type === "matching" && rule.subtype === "music");
  const plays = draft?.rules.find(rule => rule.type === "matching" && rule.subtype === "play");
  const time = draft?.rules.find(rule => rule.type === "inference" && rule.subtype === "time" && !rule.scope.file_hash && !rule.scope.source_id);
  const dirty = !!saved && !!draft && JSON.stringify(saved.current.config) !== JSON.stringify(draft);

  function override(file: FilterState["time_files"][number], update: Partial<RuleSettings> | { enabled: boolean }) {
    if (!draft || !time) return;
    const existing = draft.rules.find(rule => rule.type === "inference" && rule.subtype === "time" && rule.scope.file_hash === file.hash && (!rule.scope.source_id || rule.scope.source_id === file.source_id));
    if (existing) { edit(existing.id, update); return; }
    const rule: Rule = {
      id: `time-${file.source_id}-${file.hash}`,
      type: "inference",
      subtype: "time",
      enabled: true,
      scope: { service: "lastfm", source_id: file.source_id, file_hash: file.hash },
      settings: { ...time.settings, timezone: time.enabled ? time.settings.timezone : "" },
    };
    setDraft({ rules: [...draft.rules, rule] });
    setMessage("");
  }

  if (recordID) return <Navigate replace to={`/imports?record=${encodeURIComponent(recordID)}`} />;
  if (user?.role !== "admin") return <main className="p-6"><h1>Filters</h1><p>Administrator login required.</p></main>;

  return <main className="w-full max-w-4xl space-y-8 p-4 sm:p-8">
    <title>Filters | Koito</title>
    <header className="flex flex-wrap items-center justify-between gap-4">
      <h1>Filters</h1>
      <div className="flex items-center gap-3">
        {dirty && <button type="button" className="px-2 py-2 text-(--color-fg-secondary) hover:underline disabled:opacity-50" disabled={saving} onClick={() => { setDraft(structuredClone(saved!.current.config)); setMessage(""); }}>Discard</button>}
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
        <RuleGroup title="Music" label="Music matching" rule={music} busy={saving} onToggle={enabled => edit(music.id, { enabled })}>
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
        <RuleGroup title="Plays" label="Play matching" rule={plays} busy={saving} onToggle={enabled => edit(plays.id, { enabled })}>
          <Option label="Time tolerance" id="play-tolerance" help="Allow extra seconds between matching timestamps.">
            <span className="flex items-center gap-2"><input id="play-tolerance" aria-label="Time tolerance in seconds" className={`${input} w-24`} type="number" min="0" max="3600" step="1" disabled={saving || !plays.enabled} value={plays.settings.tolerance_seconds} onChange={event => edit(plays.id, { tolerance_seconds: Number(event.target.value) })} /><span className="text-sm text-(--color-fg-secondary)">seconds</span></span>
          </Option>
        </RuleGroup>
      </div>
    </section>}

    {plays && <section aria-labelledby="inference-title" className="space-y-3">
      <h2 id="inference-title">Inference</h2>
      <div className={card}>
        <section className="space-y-4 p-4 sm:p-5" aria-label="Time inference">
          <h3 className="text-lg font-medium normal-case tracking-normal">Time</h3>
          <Option label="Spotify timestamps" id="spotify-time" help="Estimate starts using the end time and played duration.">
            <select id="spotify-time" className={input} disabled={saving} value={plays.settings.spotify_time} onChange={event => edit(plays.id, { spotify_time: event.target.value })}>
              <option value="estimated_start">Estimated start</option><option value="end">Reported end</option>
            </select>
          </Option>
          {!!saved?.time_files.length && time && <div className="space-y-3 border-t border-(--color-bg-tertiary) pt-4">
            <Option label="Last.fm timezone" id="lastfm-time-rule" help="Interpret timestamps that omit a timezone.">
              <Switch id="lastfm-time-rule" label="Default Last.fm timezone" checked={time.enabled} disabled={saving} onChange={enabled => edit(time.id, { enabled })} />
            </Option>
            <div className={`${time.enabled ? "" : "opacity-50"} flex justify-end`}>
              <input className={input} aria-label="Last.fm timezone" list="filter-timezones" placeholder="Unknown" autoComplete="off" spellCheck={false} disabled={saving || !time.enabled} value={time.settings.timezone} onChange={event => edit(time.id, { timezone: event.target.value })} />
            </div>
            <details>
              <summary className="cursor-pointer text-sm text-(--color-fg-secondary)">File overrides</summary>
              <div className="mt-3 divide-y divide-(--color-bg-tertiary)">{saved.time_files.map(file => {
                const rule = draft?.rules.find(rule => rule.type === "inference" && rule.subtype === "time" && rule.scope.file_hash === file.hash && (!rule.scope.source_id || rule.scope.source_id === file.source_id));
                return <div key={`${file.source_id}-${file.hash}`} className="space-y-3 py-4">
                  <div className="flex items-start justify-between gap-3"><div className="min-w-0"><p className="break-all text-sm font-medium">{file.filename}</p><p className="text-sm text-(--color-fg-tertiary)">{file.records.toLocaleString()} {file.records === 1 ? "record" : "records"}</p></div><Switch label={`Override timezone for ${file.filename}`} checked={rule?.enabled || false} disabled={saving} onChange={enabled => override(file, { enabled })} /></div>
                  <div className={`${rule?.enabled ? "" : "opacity-50"} flex justify-end`}><input className={input} aria-label={`Timezone for ${file.filename}`} list="filter-timezones" placeholder="Unknown" autoComplete="off" spellCheck={false} disabled={saving || !rule?.enabled} value={rule?.settings.timezone ?? time.settings.timezone} onChange={event => override(file, { timezone: event.target.value })} /></div>
                </div>;
              })}</div>
            </details>
          </div>}
          <datalist id="filter-timezones">{["UTC", "America/Denver", "America/New_York", "America/Los_Angeles"].map(zone => <option key={zone} value={zone} />)}</datalist>
        </section>
      </div>
    </section>}
  </main>;
}

function RuleGroup({ title, label, rule, busy, onToggle, children }: { title: string; label: string; rule: Rule; busy: boolean; onToggle: (enabled: boolean) => void; children: ReactNode }) {
  return <section className="space-y-4 p-4 sm:p-5" aria-label={label}>
    <div className="flex items-center justify-between gap-4"><h3 className="text-lg font-medium normal-case tracking-normal">{title}</h3><Switch label={label} checked={rule.enabled} disabled={busy} onChange={onToggle} /></div>
    <div className={`${rule.enabled ? "" : "opacity-50"} space-y-4`}>{children}</div>
  </section>;
}

function Option({ label, id, help, children }: { label: string; id?: string; help: string; children: ReactNode }) {
  return <div className="flex flex-col justify-between gap-2 sm:flex-row sm:items-center sm:gap-6">
    <span className="inline-flex items-center gap-1 text-sm"><label htmlFor={id}>{label}</label><FieldHelp label={label}>{help}</FieldHelp></span>
    <div className="flex max-w-full items-center sm:justify-end">{children}</div>
  </div>;
}

function Switch({ id, label, checked, disabled, onChange }: { id?: string; label: string; checked: boolean; disabled?: boolean; onChange: (checked: boolean) => void }) {
  return <button id={id} type="button" role="switch" aria-label={label} aria-checked={checked} disabled={disabled} onClick={() => onChange(!checked)} className="inline-flex shrink-0 items-center gap-2 rounded px-1 py-1 disabled:cursor-default disabled:opacity-50">
    <span aria-hidden="true" className="w-6 text-left text-xs text-(--color-fg-secondary)">{checked ? "On" : "Off"}</span>
    <span aria-hidden="true" className={`${checked ? "bg-(--color-primary)" : "bg-(--color-bg-tertiary)"} relative h-5 w-9 rounded-full transition-colors`}><span className={`${checked ? "translate-x-4" : "translate-x-0"} absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-(--color-bg) shadow-sm transition-transform`} /></span>
  </button>;
}

function errorMessage(error: unknown) { return error instanceof Error ? error.message : "Could not save rules."; }
