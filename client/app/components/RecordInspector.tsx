import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { filterRequest, type FilterState } from "~/utils/filterRules";

type SourceRecord = {
  id: number;
  service: string;
  file_hash: string;
  ordinal: number;
  payload: string;
  reason: string;
  time: string;
  time_basis: string;
  values: { title: string; artist: string; album: string; time_text: string; time_meaning: string };
};
type Inspection = { record: SourceRecord; contributors: SourceRecord[]; candidates: SourceRecord[] };
const button = "rounded border border-(--color-bg-tertiary) px-3 py-2 hover:bg-(--color-bg-tertiary) disabled:opacity-50";

export default function RecordInspector({ id, onClose, onSaved }: { id: number; onClose: () => void; onSaved: () => Promise<void> }) {
  const queries = useQueryClient();
  const [inspection, setInspection] = useState<Inspection>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [target, setTarget] = useState("");

  useEffect(() => {
    let active = true;
    setInspection(undefined);
    setError("");
    setTarget("");
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

  return <section aria-label="Record inspection" className="space-y-4 rounded-lg border border-(--color-bg-tertiary) bg-(--color-bg-secondary) p-4 sm:p-5">
    <div className="flex items-center justify-between gap-3"><h2>Record #{id}</h2><button type="button" className={button} onClick={onClose}>Close</button></div>
    {error && <p role="alert" className="text-(--color-error)">{error}</p>}
    {!inspection && !error && <p role="status">Loading records...</p>}
    {inspection && <>
      <div className="space-y-2">{(inspection.contributors.length ? inspection.contributors : [inspection.record]).map(record => <RecordDetails key={record.id} record={record} />)}</div>
      {!!inspection.candidates.length && <div className="space-y-2"><h3>Possible matches</h3>{inspection.candidates.map(record => <div key={record.id} className="space-y-2"><RecordDetails record={record} /><button type="button" className={button} disabled={busy} onClick={() => decide("match", record.id)}>Match #{record.id}</button></div>)}</div>}
      <details><summary className="cursor-pointer text-sm text-(--color-fg-secondary)">Matching</summary>
        <div className="mt-3 flex flex-wrap items-center gap-3">
          <label className="flex items-center gap-2">Other record<input className="w-28 rounded border border-(--color-bg-tertiary) bg-(--color-bg) p-2" type="number" min="1" value={target} disabled={busy} onChange={event => setTarget(event.target.value)} /></label>
          {[["match", "Match"], ["split", "Split"], ["clear", "Use rules"]].map(([action, label]) => <button type="button" className={button} key={action} disabled={busy || !Number(target) || Number(target) === id} onClick={() => decide(action)}>{label}</button>)}
        </div>
      </details>
    </>}
  </section>;
}

function RecordDetails({ record }: { record: SourceRecord }) {
  let payload;
  try { payload = JSON.stringify(JSON.parse(record.payload), null, 2); } catch { payload = record.payload; }
  return <details className="rounded border border-(--color-bg-tertiary) p-3">
    <summary className="cursor-pointer break-words">#{record.id} · {record.service === "lastfm" ? "Last.fm" : "Spotify"} · {record.values.artist} · {record.values.title}</summary>
    <dl className="mt-3 grid gap-x-5 gap-y-2 text-sm sm:grid-cols-[auto_1fr]">
      <dt>Album</dt><dd>{record.values.album || "Unknown"}</dd>
      <dt>Result</dt><dd>{record.reason.replaceAll(";", ".")}</dd>
      <dt>Original time</dt><dd>{record.values.time_text} · {record.values.time_meaning}</dd>
      <dt>Used time</dt><dd>{record.time.startsWith("0001-") ? "Unknown" : record.time.replace("T", " ").replace("Z", "")} · {record.time_basis}</dd>
      <dt>Position</dt><dd>Row {record.ordinal}</dd>
    </dl>
    <pre className="mt-3 max-h-80 overflow-auto text-sm">{payload}</pre>
    <a className="underline" href={`/apis/web/v1/imports/file?hash=${record.file_hash}`}>Original file</a>
  </details>;
}

function errorMessage(error: unknown) { return error instanceof Error ? error.message : "Could not update matching."; }
