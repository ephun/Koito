import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { Link } from "react-router";
import { Download, RefreshCw, Upload } from "lucide-react";
import { useAppContext } from "~/providers/AppProvider";
import { useQueryClient } from "@tanstack/react-query";

type Source = { id: number; account: string; records: number; plays: number; first: string; last: string };
type Receipt = { id: number; source_id: number; account: string; hash: string; filename: string; status: string; error: string; created_at: number };
type RawRecord = { id: number; file_hash: string; ordinal: number; kind: string; payload: string; reason: string; play_key: string };
type Overview = { sources: Source[]; receipts: Receipt[]; policy: string };
type ImportResult = { records: number; added: number; reused: number };
type Projection = { records: number; plays: number; collisions: number };
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

  useEffect(() => { if (user?.role === "admin") refresh().catch(e => setError(String(e))); }, [user?.role]);

  async function loadRecords(selected: Source, after = 0) {
    setLoadingRecords(true); setError("");
    try {
      const page = await request<RawRecord[]>(`${base}/records?source=${selected.id}&after=${after}`);
      setRecords(previous => after ? [...previous, ...page] : page);
      setHasMore(page.length === 100); setSource(selected);
    } catch (e) { setError(String(e)); } finally { setLoadingRecords(false); }
  }

  async function rebuild() {
    setBusy(true); setError(""); setMessage("Rebuilding displayed plays from preserved records…");
    try {
      const result = await request<Projection>(`${base}/rebuild`, { method: "POST" });
      await queryClient.invalidateQueries();
      setMessage(`Rebuilt ${result.plays.toLocaleString()} plays from ${result.records.toLocaleString()} records. ${result.collisions.toLocaleString()} plays share an existing display entry.`);
      await refresh(); if (source) await loadRecords(source);
    } catch (e) { setError(String(e)); setMessage("Rebuild failed. Preserved files and records remain available."); }
    finally { setBusy(false); }
  }

  async function upload(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setBusy(true); setError("");
    let added = 0, reused = 0;
    try {
      for (const [index, file] of files.entries()) {
        setMessage(`Preserving ${file.name} (${index + 1}/${files.length})…`);
        const body = new FormData(); body.set("account", account.trim()); body.set("file", file);
        const result = await request<ImportResult>(base, { method: "POST", body });
        added += result.added; reused += result.reused;
      }
      setMessage("Files preserved. Building displayed plays…");
      const result = await request<Projection>(`${base}/rebuild`, { method: "POST" });
      await queryClient.invalidateQueries();
      setMessage(`Stored ${added.toLocaleString()} new records; reused ${reused.toLocaleString()}. The current policy identifies ${result.plays.toLocaleString()} plays, with ${result.collisions.toLocaleString()} sharing existing display entries.`);
      if (source) await loadRecords(source);
    } catch (e) {
      setError(String(e)); setMessage("Check the import history below. Successfully stored files remain preserved; failed files can be retried. Use Rebuild view if projection did not complete.");
    } finally { await refresh().catch(e => setError(String(e))); setBusy(false); }
  }

  const button = "inline-flex items-center gap-2 rounded border border-(--color-bg-tertiary) px-4 py-2 hover:bg-(--color-bg-tertiary) disabled:opacity-50 disabled:cursor-not-allowed";
  const panel = "rounded-lg border border-(--color-bg-tertiary) bg-(--color-bg-secondary) p-5";
  if (user?.role !== "admin") return <main className="w-full max-w-5xl p-6"><h1>Imports</h1><p>Log in as an administrator to manage source files and records.</p></main>;

  return <main className="w-full max-w-6xl p-4 sm:p-8 space-y-6">
    <title>Imports — Koito</title>
    <header className="flex flex-wrap items-start justify-between gap-4">
      <div><h1>Imports</h1><p className="text-(--color-fg-secondary)">Preserve your sources. Build your listening history.</p></div>
      <button className={button} disabled={busy || !overview} onClick={rebuild}><RefreshCw size={16} />Rebuild view</button>
    </header>
    <section className={panel} aria-labelledby="upload-heading">
      <h2 id="upload-heading">Spotify extended history</h2>
      <p className="mb-4 text-(--color-fg-secondary)">Upload history JSON files or the original ZIP. Every record is kept, including skips and nonmusic entries. Uploading the same file again reuses its records.</p>
      <form onSubmit={upload} className="space-y-4">
        <div>
          <label htmlFor="spotify-account" className="block">Spotify account</label>
          <input id="spotify-account" aria-describedby="account-help" className="block mt-1 w-full max-w-md rounded border border-(--color-bg-tertiary) p-2" required maxLength={200} value={account} onChange={e => setAccount(e.target.value)} placeholder="e.g. ethanjkendrick" disabled={busy} list="source-accounts" />
          <datalist id="source-accounts">{overview?.sources.map(s => <option key={s.id} value={s.account} />)}</datalist>
          <p id="account-help" className="text-sm text-(--color-fg-secondary)">Use the same account name for overlapping exports. Shared accounts should have their own names.</p>
        </div>
        <label className="block">Source files
          <input className="block mt-2 w-full" type="file" accept=".json,.zip" multiple required disabled={busy} onChange={e => setFiles(Array.from(e.target.files || []))} />
        </label>
        {files.length > 0 && <ul className="text-sm">{files.map((f, i) => <li key={i}>{f.name} · {(f.size / 1048576).toFixed(1)} MiB</li>)}</ul>}
        <p className="text-sm text-(--color-fg-secondary)">Up to 512 MiB per file and per expanded ZIP. Other source types will be added in later phases.</p>
        <button className={button} type="submit" disabled={busy || !files.length || !account.trim()}><Upload size={16} />Import {files.length > 1 ? `${files.length} files` : "file"}</button>
      </form>
    </section>
    <div aria-live="polite">{message && <p className={panel}>{message}</p>}</div>
    {error && <p role="alert" className="rounded border border-(--color-error) p-4 whitespace-pre-wrap">{error}</p>}
    <section className={panel}>
      <h2>Current policy: {overview?.policy || "spotify-v1"}</h2>
      <p>Completed music tracks count. Records with the same account, timestamp, and Spotify track identity match one play. Skips, unidentified tracks, and nonmusic remain in your records. No shared-account attribution or cross-service matching is applied yet.</p>
      <Link to="/listens?period=all_time" className="underline">View listening history</Link>
    </section>
    <section className={panel}>
      <h2>Sources</h2>
      {!overview ? <p>Loading…</p> : !overview.sources.length ? <p>Your sources will appear after the first upload.</p> : <div className="overflow-x-auto"><table className="w-full min-w-[700px] text-left [&_th]:pr-4 [&_td]:pr-4"><thead><tr><th>Account</th><th>Records</th><th>Plays</th><th>Play coverage (UTC)</th><th>Inspect</th></tr></thead><tbody>
        {overview.sources.map(s => <tr key={s.id} className="border-t border-(--color-bg-tertiary)"><td className="py-3">Spotify / {s.account}</td><td>{s.records.toLocaleString()}</td><td>{s.plays.toLocaleString()}</td><td>{s.first ? `${s.first} – ${s.last}` : "No qualifying plays"}</td><td><button className={button} disabled={busy || loadingRecords} onClick={() => loadRecords(s)}>Records</button></td></tr>)}
      </tbody></table></div>}
    </section>
    {source && <section className={panel}>
      <h2>Records · {source.account}</h2><p className="text-sm mb-3">Each entry retains its source position and original fields. Ordinals start at zero.</p>
      <div className="space-y-2">{records.map(r => <details key={r.id} className="rounded border border-(--color-bg-tertiary) p-3"><summary className="cursor-pointer">#{r.id} · {r.kind} · {r.reason} · row {r.ordinal}</summary><p className="text-sm break-all">File: {r.file_hash}{r.play_key && ` · Play: ${r.play_key}`}</p><pre className="overflow-auto max-h-80 p-3 text-sm">{formatPayload(r.payload)}</pre><a className="underline" href={`${base}/file?hash=${r.file_hash}`}>Download original source file</a></details>)}</div>
      {hasMore && <button className={`${button} mt-4`} disabled={busy || loadingRecords} onClick={() => loadRecords(source, records.at(-1)?.id)}>Load more records</button>}
    </section>}
    <section className={panel}>
      <h2>Import history</h2><p className="mb-3 text-sm text-(--color-fg-secondary)">Latest 100 upload attempts. Original files remain available even when extraction fails.</p>
      {overview?.receipts.length === 0 && <p>No uploads yet.</p>}
      <div className="space-y-3">{overview?.receipts.map(r => <article key={r.id} className="border-t border-(--color-bg-tertiary) pt-3"><div className="flex flex-wrap gap-3 justify-between"><strong className="break-all">{r.filename}</strong><a className="inline-flex items-center gap-1 underline" href={`${base}/file?hash=${r.hash}`}><Download size={14} />Original</a></div><p>{r.account} · {r.status} · {new Date(r.created_at * 1000).toLocaleString()}</p>{r.error && <p className="text-(--color-error) whitespace-pre-wrap">{r.error}</p>}</article>)}</div>
    </section>
  </main>;
}

function formatPayload(payload: string) { try { return JSON.stringify(JSON.parse(payload), null, 2); } catch { return payload; } }
