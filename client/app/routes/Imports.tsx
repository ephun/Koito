import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { Download, RefreshCw, Upload } from "lucide-react";
import { useAppContext } from "~/providers/AppProvider";
import { useQueryClient } from "@tanstack/react-query";

type Source = { id: number; account: string; records: number; plays: number; first: string; last: string };
type Receipt = { id: number; source_id: number; account: string; hash: string; filename: string; status: string; error: string; created_at: number };
type RawRecord = { id: number; file_hash: string; ordinal: number; kind: string; payload: string; reason: string; play_key: string };
type Overview = { sources: Source[]; receipts: Receipt[] };
type ImportResult = { records: number; added: number; reused: number };
const base = "/apis/web/v1/imports";

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(path, options);
  const text = await response.text();
  let data;
  try { data = JSON.parse(text); } catch { throw new Error(text || `Request failed (${response.status})`); }
  if (!response.ok) throw new Error(data.error || `Request failed (${response.status})`);
  return data as T;
}

export default function Imports() {
  const { user } = useAppContext();
  const queryClient = useQueryClient();
  const [overview, setOverview] = useState<Overview>();
  const [account, setAccount] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [source, setSource] = useState<Source>();
  const [records, setRecords] = useState<RawRecord[]>([]);
  const [loadingRecords, setLoadingRecords] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const refresh = async () => setOverview(await request<Overview>(base));

  useEffect(() => { if (user?.role === "admin") refresh().catch(e => setError(errorMessage(e))); }, [user?.role]);

  async function loadRecords(selected: Source, after = 0) {
    setLoadingRecords(true); setError("");
    try {
      const page = await request<RawRecord[]>(`${base}/records?source=${selected.id}&after=${after}`);
      setRecords(previous => after ? [...previous, ...page] : page);
      setHasMore(page.length === 100); setSource(selected);
    } catch (e) { setError(errorMessage(e)); } finally { setLoadingRecords(false); }
  }

  async function rebuild() {
    setBusy(true); setError(""); setMessage("Rebuilding…");
    try {
      await request(`${base}/rebuild`, { method: "POST" });
      await queryClient.invalidateQueries();
      setMessage("View rebuilt.");
      await refresh(); if (source) await loadRecords(source);
    } catch (e) { setError(errorMessage(e)); setMessage(""); }
    finally { setBusy(false); }
  }

  async function upload(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setBusy(true); setError("");
    let added = 0, reused = 0;
    try {
      for (const [index, file] of files.entries()) {
        setMessage(`Importing ${file.name} (${index + 1}/${files.length})…`);
        const body = new FormData(); body.set("account", account.trim()); body.set("file", file);
        const result = await request<ImportResult>(base, { method: "POST", body });
        added += result.added; reused += result.reused;
      }
      setMessage("Updating…");
      await request(`${base}/rebuild`, { method: "POST" });
      await queryClient.invalidateQueries();
      setMessage(`${added.toLocaleString()} records imported${reused ? ` · ${reused.toLocaleString()} already imported` : ""}.`);
      if (source) await loadRecords(source);
    } catch (e) {
      setError(errorMessage(e)); setMessage("");
    } finally { await refresh().catch(e => setError(errorMessage(e))); setBusy(false); }
  }

  const button = "inline-flex items-center gap-2 rounded border border-(--color-bg-tertiary) px-4 py-2 hover:bg-(--color-bg-tertiary) disabled:opacity-50 disabled:cursor-not-allowed";
  const panel = "rounded-lg border border-(--color-bg-tertiary) bg-(--color-bg-secondary) p-5";
  if (user?.role !== "admin") return <main className="w-full max-w-5xl p-6"><h1>Imports</h1><p>Administrator login required.</p></main>;

  return <main className="w-full max-w-6xl p-4 sm:p-8 space-y-6">
    <title>Imports — Koito</title>
    <header className="flex flex-wrap items-start justify-between gap-4">
      <h1>Imports</h1>
      {overview?.sources.some(s => s.records > 0) && <button className={button} disabled={busy} onClick={rebuild}><RefreshCw size={16} />Rebuild view</button>}
    </header>
    <section className={panel} aria-labelledby="upload-heading">
      <h2 id="upload-heading">Import file</h2>
      <form onSubmit={upload} className="space-y-4">
        <div>
          <label htmlFor="source-account" className="block">Account</label>
          <input id="source-account" className="block mt-1 w-full max-w-md rounded border border-(--color-bg-tertiary) p-2" required maxLength={200} value={account} onChange={e => setAccount(e.target.value)} disabled={busy} list="source-accounts" />
          <datalist id="source-accounts">{overview?.sources.map(s => <option key={s.id} value={s.account} />)}</datalist>
        </div>
        <label className="block">File
          <input className="block mt-2 w-full" type="file" multiple required disabled={busy} onChange={e => setFiles(Array.from(e.target.files || []))} />
        </label>
        {files.length > 0 && <ul className="text-sm">{files.map((f, i) => <li key={i}>{f.name} · {(f.size / 1048576).toFixed(1)} MiB</li>)}</ul>}
        <button className={button} type="submit" disabled={busy || !files.length || !account.trim()}><Upload size={16} />Import {files.length > 1 ? `${files.length} files` : "file"}</button>
      </form>
    </section>
    <div aria-live="polite">{message && <p className={panel}>{message}</p>}</div>
    {error && <p role="alert" className="rounded border border-(--color-error) p-4 whitespace-pre-wrap">{error}</p>}
    {!!overview?.sources.some(s => s.records > 0) && <section className={panel}>
      <h2>Sources</h2>
      <div className="overflow-x-auto"><table className="w-full min-w-[700px] text-left [&_th]:pr-4 [&_td]:pr-4"><thead><tr><th>Account</th><th>Records</th><th>Plays</th><th>Dates (UTC)</th><th>Inspect</th></tr></thead><tbody>
        {overview.sources.filter(s => s.records > 0).map(s => <tr key={s.id} className="border-t border-(--color-bg-tertiary)"><td className="py-3">{s.account}</td><td>{s.records.toLocaleString()}</td><td>{s.plays.toLocaleString()}</td><td>{s.first ? `${s.first} – ${s.last}` : "—"}</td><td><button className={button} disabled={busy || loadingRecords} onClick={() => loadRecords(s)}>Records</button></td></tr>)}
      </tbody></table></div>
    </section>}
    {source && <section className={panel}>
      <h2>Records · {source.account}</h2>
      <div className="space-y-2">{records.map(r => <details key={r.id} className="rounded border border-(--color-bg-tertiary) p-3"><summary className="cursor-pointer">#{r.id} · {r.kind} · {r.reason} · row {r.ordinal}</summary><p className="text-sm break-all">File: {r.file_hash}{r.play_key && ` · Play: ${r.play_key}`}</p><pre className="overflow-auto max-h-80 p-3 text-sm">{formatPayload(r.payload)}</pre><a className="underline" href={`${base}/file?hash=${r.file_hash}`}>Download original source file</a></details>)}</div>
      {hasMore && <button className={`${button} mt-4`} disabled={busy || loadingRecords} onClick={() => loadRecords(source, records.at(-1)?.id)}>Load more</button>}
    </section>}
    {!!overview?.receipts.length && <section className={panel}>
      <h2>Import history</h2>
      <div className="space-y-3">{overview?.receipts.map(r => <article key={r.id} className="border-t border-(--color-bg-tertiary) pt-3"><div className="flex flex-wrap gap-3 justify-between"><strong className="break-all">{r.filename}</strong><a className="inline-flex items-center gap-1 underline" href={`${base}/file?hash=${r.hash}`}><Download size={14} />Original</a></div><p>{r.account} · {r.status} · {new Date(r.created_at * 1000).toLocaleString()}</p>{r.error && <p className="text-(--color-error) whitespace-pre-wrap">{r.error}</p>}</article>)}</div>
    </section>}
  </main>;
}

function formatPayload(payload: string) { try { return JSON.stringify(JSON.parse(payload), null, 2); } catch { return payload; } }
function errorMessage(error: unknown) { return error instanceof Error ? error.message : String(error); }
