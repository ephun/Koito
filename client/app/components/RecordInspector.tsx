import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { filterRequest, sourceName, type FilterState } from "~/utils/filterRules";
import FieldHelp from "./FieldHelp";

type SourceRecord = {
  id: number;
  service: string;
  file_hash: string;
  ordinal: number;
  payload: string;
  reason: string;
  time: string;
  time_basis: string;
  values: { title: string; artist: string; album: string; time_text: string; time_meaning: string; reason?: string };
};
type Attribution = { listener: string; basis: string; share_percent?: number };
type Contribution = {
  key: string; reported: number | null; available: number; attributed: number; observed: number;
  overlap: number; last_played: number; estimated: number; undated: number;
  attribution: Attribution; reason: string; from: string; until: string; records: number[];
};
type Inspection = { snapshots?: { id: number; name: string; kind: string; position: number; metadata: unknown }[]; source_genres?: { value: string; field: string }[]; record: SourceRecord; contributors: SourceRecord[]; candidates: SourceRecord[]; attribution?: Attribution; contributions: Contribution[]; filter_issues?: { filter_id: string; name: string; path: string; reason: string }[] };
const button = "rounded border border-(--color-bg-tertiary) px-3 py-2 hover:bg-(--color-bg-tertiary) disabled:opacity-50";

export default function RecordInspector({ id, onClose, onSaved }: { id: number; onClose: () => void; onSaved: () => Promise<void> }) {
  const queries = useQueryClient();
  const [inspection, setInspection] = useState<Inspection>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [target, setTarget] = useState("");
  const [pair, setPair] = useState<{ left: number; right: number; music: boolean; play: boolean; issues: { name: string; reason: string }[] }>();

  useEffect(() => {
    let active = true;
    setInspection(undefined);
    setError("");
    setTarget("");
    setPair(undefined);
    filterRequest<Inspection>(`/record?id=${id}`).then(value => {
      if (!active) return;
      setInspection(value);
      const partner = value.contributors.find(record => record.id !== id) || value.candidates[0];
      if (partner) setTarget(String(partner.id));
    }).catch(error => { if (active) setError(errorMessage(error)); });
    return () => { active = false; };
  }, [id]);

  async function decide(action: string, partner = Number(target)) {
    if (!partner || partner === id || busy) return;
    setBusy(true);
    setError("");
    try {
      const state = await filterRequest<FilterState>();
      await filterRequest("/save", { base_version: state.current.id, config: state.current.config, decision: { left: id, right: partner, action } });
      setInspection(await filterRequest<Inspection>(`/record?id=${id}`));
      await queries.invalidateQueries();
      await onSaved();
    } catch (error) { setError(errorMessage(error)); }
    finally { setBusy(false); }
  }

  async function compare() {
    if (!Number(target) || busy) return;
    setError("");setBusy(true);setPair(undefined);
    try { setPair(await filterRequest(`/pair?left=${id}&right=${Number(target)}`)); }
    catch (error) { setError(errorMessage(error)); }
    finally { setBusy(false); }
  }

  return <section aria-label="Record inspection" className="space-y-4 rounded-lg border border-(--color-bg-tertiary) bg-(--color-bg-secondary) p-4 sm:p-5">
    <div className="flex items-center justify-between gap-3"><h2>Record #{id}</h2><button type="button" className={button} onClick={onClose}>Close</button></div>
    {error && <p role="alert" className="text-(--color-error)">{error}</p>}
    {!inspection && !error && <p role="status">Loading records...</p>}
    {inspection && <>
      {!!inspection.filter_issues?.length && <details className="rounded border border-(--color-bg-tertiary) p-3">
        <summary className="cursor-pointer text-sm">Unresolved filters<FieldHelp label="Unresolved filters">Check enabled filters using this record's fields.</FieldHelp></summary>
        <ul className="mt-2 space-y-2 text-sm">{inspection.filter_issues.map((issue, index) => <li key={`${issue.filter_id}:${index}`} className="break-words"><a className="underline" href="/filters">{issue.name || "Filter"}</a>: {issue.reason}</li>)}</ul>
      </details>}
      {!!inspection.source_genres?.length && <details className="rounded border border-(--color-bg-tertiary) p-3">
        <summary className="cursor-pointer text-sm">Source genres</summary>
        <ul className="mt-2 space-y-2 text-sm">{inspection.source_genres.map((genre, index) => <li key={index} className="break-words">
          <span className="whitespace-pre-wrap">{genre.value || "Empty"}</span>
          <span className="ml-2 text-(--color-fg-secondary)">{genre.field.replace(/^raw:\/fields\//, "").replace(/\/\$plist_repeated\//, " / ")}</span>
        </li>)}</ul>
      </details>}
      {!!inspection.snapshots?.length && <details className="rounded border border-(--color-bg-tertiary) p-3">
        <summary className="cursor-pointer text-sm">Snapshot</summary>
        {inspection.snapshots.map(snapshot => <div className="mt-2 space-y-2" key={snapshot.id}>
          <p className="text-sm">{snapshot.name} · Item {snapshot.position + 1}</p>
          <pre className="max-h-80 overflow-auto text-sm">{JSON.stringify(snapshot.metadata, null, 2)}</pre>
        </div>)}
      </details>}
      {inspection.attribution && <p className="text-sm">Listener: {listenerName(inspection.attribution)}{inspection.attribution.basis === "heuristic" && " (Heuristic)"}</p>}
      {!!inspection.contributions?.length && <div className="space-y-3">{inspection.contributions.map(contribution => <div key={contribution.key} className="space-y-3 rounded border border-(--color-bg-tertiary) p-3">
        <h3 className="font-medium">Count</h3>
        <dl className="grid grid-cols-[1fr_auto] gap-x-5 gap-y-2 text-sm">
          <dt className="flex items-center gap-1">Reported<FieldHelp label="Reported count">The largest valid cumulative count in the source records.</FieldHelp></dt><dd>{contribution.reported?.toLocaleString() ?? "Unknown"}</dd>
          <dt className="flex items-center gap-1">Interpreted<FieldHelp label="Interpreted count">The amount produced by the selected Count filter.</FieldHelp></dt><dd>{contribution.available.toLocaleString()}</dd>
          <dt>Assigned to me</dt><dd>{contribution.attributed.toLocaleString()}</dd>
          {!!contribution.observed && <><dt>Observed</dt><dd>{contribution.observed.toLocaleString()}</dd></>}
          {!!contribution.overlap && <><dt>Matched endpoints</dt><dd>{contribution.overlap.toLocaleString()}</dd></>}
          {!!contribution.last_played && <><dt>Last played</dt><dd>{contribution.last_played.toLocaleString()}</dd></>}
          {!!contribution.estimated && <><dt>Estimated</dt><dd>{contribution.estimated.toLocaleString()}</dd></>}
          <dt>Undated</dt><dd>{contribution.undated.toLocaleString()}</dd>
          <dt>Listener</dt><dd>{listenerName(contribution.attribution)}</dd>
        </dl>
        {contribution.attribution.basis === "heuristic" && <p className="text-sm">Heuristic attribution</p>}
        {contribution.reason !== "count inference" && <p className="text-sm">{contribution.reason}</p>}
        <details><summary className="cursor-pointer text-sm">Supporting records</summary><div className="mt-2 flex flex-wrap gap-3">{contribution.records.map(record => <a className="underline" key={record} href={`/imports?record=${record}`}>Record #{record}</a>)}</div>
          {contribution.from && !contribution.from.startsWith("0001-") && <p className="mt-2 text-sm">From {new Date(contribution.from).toLocaleString()}</p>}
          {contribution.until && !contribution.until.startsWith("0001-") && <p className="text-sm">Until {new Date(contribution.until).toLocaleString()}</p>}
        </details>
        <a className="text-sm underline" href="/filters">Edit filters</a>
      </div>)}</div>}
      <div className="space-y-2">{(inspection.contributors.length ? inspection.contributors : [inspection.record]).map(record => <RecordDetails key={record.id} record={record} />)}</div>
      {!!inspection.candidates.length && <div className="space-y-2"><h3>Possible matches</h3>{inspection.candidates.map(record => <div key={record.id} className="space-y-2"><RecordDetails record={record} /><button type="button" className={button} disabled={busy} onClick={() => decide("match", record.id)}>Match #{record.id}</button></div>)}</div>}
      {inspection.record.values.reason === "counted" && <details><summary className="cursor-pointer text-sm text-(--color-fg-secondary)">Matching</summary>
        <div className="mt-3 flex flex-wrap items-center gap-3">
          <label className="flex items-center gap-2">Other record<input className="w-28 rounded border border-(--color-bg-tertiary) bg-(--color-bg) p-2" type="number" min="1" value={target} disabled={busy} onChange={event => { setTarget(event.target.value); setPair(undefined); }} /></label>
          <button type="button" className={button} disabled={busy || !Number(target) || Number(target) === id} onClick={compare}>Compare</button>
          {[["match", "Match"], ["split", "Split"], ["clear", "Use filters"]].map(([action, label]) => <button type="button" className={button} key={action} disabled={busy || !Number(target) || Number(target) === id} onClick={() => decide(action)}>{label}</button>)}
        </div>
        {pair && <div className="mt-3 space-y-2 text-sm" role="status"><p>Music matching: {pair.music ? "TRUE" : "FALSE"}</p><p>Play matching: {pair.play ? "TRUE" : "FALSE"}</p>{pair.issues.map((issue,index) => <p key={index} className="break-words">{issue.name || "Filter"}: {issue.reason}</p>)}</div>}
      </details>}
    </>}
  </section>;
}

function RecordDetails({ record }: { record: SourceRecord }) {
  let payload;
  try { payload = JSON.stringify(JSON.parse(record.payload), null, 2); } catch { payload = record.payload; }
  return <details className="rounded border border-(--color-bg-tertiary) p-3">
    <summary className="cursor-pointer break-words">#{record.id} · {sourceName(record.service)}{record.values.artist && ` · ${record.values.artist}`}{record.values.title && ` · ${record.values.title}`}</summary>
    <dl className="mt-3 grid gap-x-5 gap-y-2 text-sm sm:grid-cols-[auto_1fr]">
      <dt>Album</dt><dd>{record.values.album || "Unknown"}</dd>
      <dt>Result</dt><dd>{record.reason.replaceAll(";", ".")}</dd>
      {!!record.values.time_text && <><dt>Original time</dt><dd>{record.values.time_text} · {record.values.time_meaning}</dd></>}
      {!record.time.startsWith("0001-") && <><dt>Used time</dt><dd>{record.time.replace("T", " ").replace("Z", "")} · {record.time_basis}</dd></>}
      <dt>Position</dt><dd>Row {record.ordinal}</dd>
    </dl>
    <pre className="mt-3 max-h-80 overflow-auto text-sm">{payload}</pre>
    <a className="underline" href={`/apis/web/v1/imports/file?hash=${record.file_hash}`}>Original file</a>
  </details>;
}

function errorMessage(error: unknown) { return error instanceof Error ? error.message : "Could not update matching."; }
function listenerName(attribution: Attribution) { return attribution.listener === "self" ? "Me" : attribution.listener === "other" ? "Someone else" : attribution.listener === "partial" ? `${attribution.share_percent ?? 0}%` : "Unknown"; }
