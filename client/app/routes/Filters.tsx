import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAppContext } from "~/providers/AppProvider";

type Settings = { case_sensitive: boolean; match_missing_album: boolean; tolerance_seconds: number; spotify_time?: string; preferred_source?: string; timezone: string };
type Rule = { id: string; type: string; subtype: string; enabled: boolean; scope: { service?: string; source_id?: number; file_hash?: string }; settings: Settings };
type Config = { rules: Rule[] };
type Version = { id: number; parent_id: number; config: Config; label: string; created_at: number };
type Decision = { id: number; left: number; right: number; action: string; created_at: number };
type State = { current: Version; history: Version[]; time_files: { hash: string; filename: string; source_id: number; records: number }[]; decisions: Decision[] };
type Change = { base_version: number; config: Config; restore_version?: number; decision?: { left: number; right: number; action: string; undo_id?: number }; token?: string };
type Preview = { token: string; before: number; after: number; records: number; unresolved: number; ambiguous: number; changed: number; examples: { before: number[][]; after: number[]; title: string; artist: string }[] };
type Play = { key: string; record_id: number; title: string; artist: string; album: string; time: string; records: number; reason: string };
type RecordData = { id: number; service: string; file_hash: string; ordinal: number; parser: string; payload: string; reason: string; time: string; time_basis: string; values: { title: string; artist: string; album: string; time_text: string; time_meaning: string; precision_ns: number; origin: string } };
type Inspection = { record: RecordData; contributors: RecordData[]; candidates: RecordData[] };
const base = "/apis/web/v1/filters";
const button = "rounded border border-(--color-bg-tertiary) px-3 py-2 hover:bg-(--color-bg-tertiary) disabled:opacity-50 disabled:cursor-not-allowed";
const input = "rounded border border-(--color-bg-tertiary) bg-(--color-bg) px-3 py-2 min-w-0";
const panel = "rounded-lg border border-(--color-bg-tertiary) bg-(--color-bg-secondary) p-4 sm:p-5 space-y-4";
async function request<T>(path: string, body?: unknown): Promise<T> {
  const response = await fetch(base + path, body === undefined ? undefined : { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
  const text = await response.text();
  let data; try { data = JSON.parse(text); } catch { throw new Error(text || `Request failed (${response.status})`); }
  if (!response.ok) throw new Error(data.error || `Request failed (${response.status})`);
  return data as T;
}

export default function Filters() {
  const { user } = useAppContext();
  const queries = useQueryClient();
  const [state, setState] = useState<State>();
  const [config, setConfig] = useState<Config>();
  const [preview, setPreview] = useState<Preview>();
  const [pending, setPending] = useState<Change>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [plays, setPlays] = useState<Play[]>([]);
  const [query, setQuery] = useState("");
  const [ambiguous, setAmbiguous] = useState(false);
  const [more, setMore] = useState(false);
  const [inspection, setInspection] = useState<Inspection>();
  const [recordID, setRecordID] = useState("");
  const [left, setLeft] = useState("");
  const [right, setRight] = useState("");

  async function loadPlays(after = 0) {
    const rows = await request<Play[]>(`/plays?after=${after}&q=${encodeURIComponent(query)}&ambiguous=${ambiguous}`);
    setPlays(previous => after ? [...previous, ...rows] : rows); setMore(rows.length === 100);
  }
  async function refresh() {
    const next = await request<State>(""); setState(next); setConfig(structuredClone(next.current.config));
    await loadPlays();
  }
  useEffect(() => {
    if (user?.role !== "admin") return;
    refresh().catch(e => setError(errorMessage(e)));
    const id = new URLSearchParams(window.location.search).get("record");
    if (id) void inspect(Number(id));
  }, [user?.role]);

  async function perform(work: () => Promise<void>) {
    setBusy(true); setError(""); setMessage("");
    try { await work(); } catch (e) {
      const message = errorMessage(e); setError(message);
      if (message === "Data changed. Preview again.") {
        setPreview(undefined); setPending(undefined);
        try { setState(await request<State>("")); } catch { /* Keep the original action error. */ }
      }
    } finally { setBusy(false); }
  }
  function edit(id: string, update: Partial<Settings> | { enabled: boolean }) {
    setConfig(previous => previous && ({ rules: previous.rules.map(rule => rule.id !== id ? rule : "enabled" in update ? { ...rule, enabled: update.enabled } : { ...rule, settings: { ...rule.settings, ...update } }) }));
    setPreview(undefined); setPending(undefined);
  }
  async function showPreview(change?: Partial<Change>) {
    if (!state || !config) return;
    const next: Change = { base_version: state.current.id, config, ...change };
    await perform(async () => { setPreview(undefined); setPending(undefined); const result = await request<Preview>("/preview", next); setPending(next); setPreview(result); });
  }
  async function apply() {
    if (!pending || !preview) return;
    await perform(async () => {
      await request("/apply", { ...pending, token: preview.token }); setPreview(undefined); setPending(undefined);
      await queries.invalidateQueries(); await refresh();
      if (inspection) setInspection(await request<Inspection>(`/record?id=${inspection.record.id}`));
      setMessage("Applied.");
    });
  }
  async function inspect(id: number) {
    await perform(async () => { const result = await request<Inspection>(`/record?id=${id}`); setInspection(result); setLeft(String(id)); setRight(""); });
  }
  function timezone(hash: string, sourceID: number, zone: string) {
    if (!config) return;
    const id = `time-${sourceID}-${hash}`;
    const existing = config.rules.find(r => r.scope.file_hash === hash && (!r.scope.source_id || r.scope.source_id === sourceID) && r.subtype === "time");
    if (existing) edit(existing.id, { timezone: zone });
    else {
      setConfig({ rules: [...config.rules, { id, type: "inference", subtype: "time", enabled: true, scope: { service: "lastfm", source_id: sourceID, file_hash: hash }, settings: { case_sensitive: true, match_missing_album: true, tolerance_seconds: 0, timezone: zone } }] });
      setPreview(undefined); setPending(undefined);
    }
  }
  const music = config?.rules.find(r => r.type === "matching" && r.subtype === "music");
  const matching = config?.rules.find(r => r.type === "matching" && r.subtype === "play");
  const timeRule = config?.rules.find(r => r.type === "inference" && r.subtype === "time" && !r.scope.file_hash && !r.scope.source_id);
  if (user?.role !== "admin") return <main className="p-6"><h1>Filters</h1><p>Administrator login required.</p></main>;
  return <main className="w-full max-w-6xl p-4 sm:p-8 space-y-6">
    <title>Filters — Koito</title>
    <header className="flex flex-wrap justify-between gap-3"><h1>Filters</h1><div className="flex gap-2">
      <button className={button} disabled={busy || !state?.current.parent_id} onClick={() => showPreview({ restore_version: state?.current.parent_id })}>Undo rules</button>
      <button className={button} disabled={busy || !config} onClick={() => showPreview()}>Preview</button>
    </div></header>
    {error && <p role="alert" className="rounded border border-(--color-error) p-4 whitespace-pre-wrap">{error}</p>}
    <div aria-live="polite">{busy ? <p>Working…</p> : message && <p>{message}</p>}</div>
    {preview && <section className={panel} aria-label="Preview">
      <div className="flex flex-wrap items-center justify-between gap-3"><h2>Preview{pending?.restore_version ? ` · Restore rules ${pending.restore_version}` : pending?.decision ? " · Manual change" : ""}</h2><div className="flex gap-2"><button className={button} disabled={busy} onClick={() => { setPreview(undefined); setPending(undefined); }}>Cancel</button><button className={button} disabled={busy} onClick={apply}>Apply</button></div></div>
      <dl className="flex flex-wrap gap-x-8 gap-y-3">{[["Plays", `${preview.before.toLocaleString()} → ${preview.after.toLocaleString()}`], ["Changed", preview.changed.toLocaleString()], ["Ambiguous records", preview.ambiguous.toLocaleString()], ["Uncounted records", preview.unresolved.toLocaleString()]].map(([name, value]) => <div key={name}><dt className="text-(--color-fg-secondary)">{name}</dt><dd>{value}</dd></div>)}</dl>
      {!!preview.examples.length && <ul className="space-y-2">{preview.examples.map((example, i) => <li key={i} className="border-t border-(--color-bg-tertiary) pt-2"><strong>{example.artist} · {example.title}</strong><div className="flex flex-wrap items-center gap-2">{example.before.map((ids, n) => <span key={n}>[{ids.map(id => <button className="underline px-1" key={id} onClick={() => inspect(id)}>#{id}</button>)}]</span>)}<span>→</span><span>[{example.after.map(id => <button className="underline px-1" key={id} onClick={() => inspect(id)}>#{id}</button>)}]</span></div></li>)}</ul>}
    </section>}
    {music && matching && <fieldset disabled={busy} className="grid md:grid-cols-2 gap-6">
      <section className={panel}><h2>Music matching</h2><Toggle label="Enabled" checked={music.enabled} onChange={enabled => edit(music.id, { enabled })} /><Toggle label="Case sensitive" checked={music.settings.case_sensitive} onChange={case_sensitive => edit(music.id, { case_sensitive })} /><Toggle label="Match missing albums" checked={music.settings.match_missing_album} onChange={match_missing_album => edit(music.id, { match_missing_album })} /></section>
      <section className={panel}><h2>Play matching</h2><Toggle label="Enabled" checked={matching.enabled} onChange={enabled => edit(matching.id, { enabled })} />
        <label className="flex flex-wrap items-center justify-between gap-2">Time tolerance (seconds)<input className={`${input} w-28`} type="number" min="0" max="3600" value={matching.settings.tolerance_seconds} onChange={e => edit(matching.id, { tolerance_seconds: Number(e.target.value) })} /></label>
        <label className="flex flex-wrap items-center justify-between gap-2">Spotify time<select className={input} value={matching.settings.spotify_time} onChange={e => edit(matching.id, { spotify_time: e.target.value })}><option value="estimated_start">Estimated start</option><option value="end">Reported end</option></select></label>
        <label className="flex flex-wrap items-center justify-between gap-2">Displayed source<select className={input} value={matching.settings.preferred_source} onChange={e => edit(matching.id, { preferred_source: e.target.value })}><option value="spotify">Spotify</option><option value="lastfm">Last.fm</option></select></label>
      </section>
    </fieldset>}
    {!!state?.time_files.length && <section className={panel}><h2>Timezones</h2>
      {timeRule && <div className="flex flex-wrap gap-4 items-center"><Toggle label="Enabled" checked={timeRule.enabled} onChange={enabled => edit(timeRule.id, { enabled })} /><label className="flex flex-wrap gap-3 items-center">Default Last.fm timezone<input className={input} disabled={busy} placeholder="Unknown" value={timeRule.settings.timezone} onChange={e => edit(timeRule.id, { timezone: e.target.value })} list="timezones" /></label></div>}
      <datalist id="timezones">{["UTC", "America/Denver", "America/New_York", "America/Los_Angeles"].map(zone => <option key={zone} value={zone} />)}</datalist>
      {state.time_files.map(file => { const rule = config?.rules.find(r => r.subtype === "time" && r.scope.file_hash === file.hash && (!r.scope.source_id || r.scope.source_id === file.source_id)); return <div key={`${file.source_id}-${file.hash}`} className="flex flex-wrap justify-between gap-3 border-t border-(--color-bg-tertiary) pt-3"><div className="min-w-0 break-all"><strong>{file.filename}</strong><div>{file.records.toLocaleString()} records</div></div><label className="flex flex-wrap gap-2 items-center">Timezone<input className={input} disabled={busy} aria-label={`Timezone for ${file.filename}`} placeholder={rule ? "Unknown" : (timeRule?.enabled && timeRule.settings.timezone) || "Unknown"} value={rule?.settings.timezone || ""} onChange={e => timezone(file.hash, file.source_id, e.target.value)} list="timezones" /></label>{rule && <Toggle label="Enabled" checked={rule.enabled} onChange={enabled => edit(rule.id, { enabled })} />}</div>; })}
    </section>}
    <section className={panel}><h2>Plays</h2><form className="flex flex-wrap gap-3 items-center" onSubmit={e => { e.preventDefault(); void perform(() => loadPlays()); }}><input className={input} aria-label="Search plays" placeholder="Artist or track" value={query} onChange={e => setQuery(e.target.value)} /><Toggle label="Ambiguous only" checked={ambiguous} onChange={setAmbiguous} /><button className={button} disabled={busy}>Search</button></form>
      <div className="overflow-auto"><table className="w-full min-w-[650px] text-left [&_td]:pr-4 [&_th]:pr-4"><thead><tr><th>Music</th><th>Time (UTC)</th><th>Records</th><th></th></tr></thead><tbody>{plays.map(play => <tr key={play.key} className="border-t border-(--color-bg-tertiary)"><td className="py-3"><strong>{play.title}</strong><div>{play.artist} · {play.album || "—"}</div>{play.reason.includes("ambiguous") && <div>Ambiguous</div>}</td><td>{formatTime(play.time)}</td><td>{play.records}</td><td><button className={button} disabled={busy} onClick={() => inspect(play.record_id)}>Inspect</button></td></tr>)}</tbody></table></div>
      {more && <button className={button} disabled={busy} onClick={() => perform(() => loadPlays(plays.at(-1)?.record_id))}>Load more</button>}
      <form className="flex flex-wrap gap-3 items-center" onSubmit={e => { e.preventDefault(); void inspect(Number(recordID)); }}><label className="flex gap-2 items-center">Record<input className={`${input} w-32`} type="number" min="1" value={recordID} onChange={e => setRecordID(e.target.value)} /></label><button className={button} disabled={busy || !Number(recordID)}>Inspect record</button></form>
    </section>
    {inspection && <section className={panel}><div className="flex justify-between gap-3"><h2>Record #{inspection.record.id}</h2><button className={button} onClick={() => setInspection(undefined)}>Close</button></div>
      <div className="space-y-3">{(inspection.contributors.length ? inspection.contributors : [inspection.record]).map(record => <RecordDetails key={record.id} record={record} />)}</div>
      {!!inspection.candidates.length && <div><h3>Possible matches</h3><div className="space-y-3 mt-3">{inspection.candidates.map(record => <div key={record.id}><RecordDetails record={record} /><button className={`${button} mt-2`} disabled={busy} onClick={() => { setLeft(String(inspection.record.id)); setRight(String(record.id)); }}>Select #{record.id}</button></div>)}</div></div>}
      <div className="flex flex-wrap gap-3 items-center"><label className="flex gap-2 items-center">First record<input className={`${input} w-28`} type="number" min="1" value={left} onChange={e => setLeft(e.target.value)} /></label><label className="flex gap-2 items-center">Second record<input className={`${input} w-28`} type="number" min="1" value={right} onChange={e => setRight(e.target.value)} /></label>{["match", "split", "clear"].map(action => <button className={button} disabled={busy || !Number(left) || !Number(right) || left === right} key={action} onClick={() => showPreview({ config: state!.current.config, decision: { left: Number(left), right: Number(right), action } })}>{action === "clear" ? "Use rules" : action === "match" ? "Match" : "Split"}</button>)}</div>
    </section>}
    {!!state?.history.length && <details className={panel}><summary className="cursor-pointer">Rule history · Version {state.current.id}</summary><div className="space-y-3">{state.history.map(version => <div key={version.id} className="flex flex-wrap justify-between items-center gap-3 border-t border-(--color-bg-tertiary) pt-3"><span>#{version.id} · {version.label} · {new Date(version.created_at * 1000).toLocaleString()}</span><button className={button} disabled={busy || version.id === state.current.id} onClick={() => showPreview({ restore_version: version.id })}>Restore rules</button></div>)}</div></details>}
    {!!state?.decisions.length && <details className={panel}><summary className="cursor-pointer">Manual changes</summary><div className="space-y-3">{state.decisions.map((decision, i) => <div key={decision.id} className="flex flex-wrap justify-between items-center gap-3 border-t border-(--color-bg-tertiary) pt-3"><span>#{decision.id} · {decision.action} · Records #{decision.left}, #{decision.right}</span><button className={button} disabled={busy || state.decisions.slice(0, i).some(d => d.left === decision.left && d.right === decision.right)} onClick={() => showPreview({ config: state.current.config, decision: { left: 0, right: 0, action: "", undo_id: decision.id } })}>Undo</button></div>)}</div></details>}
  </main>;
}

function Toggle({ label, checked, onChange }: { label: string; checked: boolean; onChange: (checked: boolean) => void }) { return <label className="flex gap-3 items-center"><input type="checkbox" checked={checked} onChange={e => onChange(e.target.checked)} />{label}</label>; }
function RecordDetails({ record }: { record: RecordData }) {
  let payload; try { payload = JSON.stringify(JSON.parse(record.payload), null, 2); } catch { payload = record.payload; }
  return <details className="rounded border border-(--color-bg-tertiary) p-3"><summary className="cursor-pointer">#{record.id} · {record.service === "lastfm" ? "Last.fm" : "Spotify"} · {record.values.artist} · {record.values.title} · {record.values.album || "—"}</summary><dl className="mt-3 grid sm:grid-cols-[auto_1fr] gap-x-5 gap-y-2 text-sm"><dt>Result</dt><dd>{record.reason}</dd><dt>Original time</dt><dd>{record.values.time_text} · {record.values.time_meaning}</dd><dt>Used time</dt><dd>{formatTime(record.time)} · {record.time_basis}</dd><dt>Precision</dt><dd>{record.values.precision_ns / 1e9} seconds</dd><dt>Source</dt><dd>{record.values.origin} · row {record.ordinal} · {record.parser}</dd></dl><pre className="overflow-auto max-h-80 mt-3 text-sm">{payload}</pre><a className="underline" href={`/apis/web/v1/imports/file?hash=${record.file_hash}`}>Original file</a></details>;
}
function errorMessage(error: unknown) { return error instanceof Error ? error.message : String(error); }
function formatTime(time: string) { return time.startsWith("0001-") ? "—" : time.replace("T", " ").replace("Z", ""); }
