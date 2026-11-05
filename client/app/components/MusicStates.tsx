import { useEffect, useState } from "react";
import { Link } from "react-router";
import { useAppContext } from "~/providers/AppProvider";

type MusicState = { kind: string; name: string; records: number[]; quantity: number };

export default function MusicStates({ kind, id }: { kind: "track" | "album"; id: number }) {
  const { user } = useAppContext();
  const [states, setStates] = useState<MusicState[]>([]);
  useEffect(() => {
    setStates([]);
    if (user?.role !== "admin") return;
    const controller = new AbortController();
    fetch(`/apis/web/v1/imports/music?${kind}=${id}`, { signal: controller.signal })
      .then(response => { if (!response.ok) throw new Error("Could not load music sources."); return response.json() as Promise<MusicState[]>; })
      .then(setStates).catch(error => { if (error.name !== "AbortError") console.error(error); });
    return () => controller.abort();
  }, [kind, id, user?.role]);
  if (!states.length) return null;
  const playlists = states.filter(state => state.kind === "playlist");
  const music = states.filter(state => state.kind !== "playlist");
  const stateLink = (state: MusicState) => {
      const label = state.kind === "playlist" ? state.name : ({ saved_tracks: kind === "album" ? "Saved tracks" : "Saved", saved_albums: "Saved album", catalog: "Navidrome library", owned: "Owned", wanted: "Wanted", inventory: "Inventory", contributions: "Contributed" } as Record<string, string>)[state.kind] || state.kind;
      return <Link key={`${state.kind}:${state.name}`} className="inline-block max-w-full rounded-xs bg-(--color-bg-secondary) px-2 py-0.5 text-sm leading-5 [overflow-wrap:anywhere] hover:underline" to={`/imports?record=${state.records[0]}`} title="View source records">
        {label}{state.kind === "owned" && state.quantity > 1 ? ` ×${state.quantity}` : ""}
      </Link>;
  };
  return <div className="mt-2 min-w-0 max-w-full space-y-2" aria-label="Music sources">
    {!!music.length && <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">{music.map(stateLink)}</div>}
    {!!playlists.length && <details className="min-w-0 max-w-full">
      <summary className="w-fit cursor-pointer text-sm underline underline-offset-2">Playlists</summary>
      <div className="mt-2 flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1">{playlists.map(stateLink)}</div>
    </details>}
  </div>;
}
