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
  return <div className="mt-2 flex flex-wrap items-baseline gap-x-2 gap-y-1" aria-label="Music sources">
    {states.map(state => {
      const label = state.kind === "playlist" ? state.name : ({ saved_tracks: "Saved", saved_albums: "Saved album", owned: "Owned", wanted: "Wanted", inventory: "Inventory", contributions: "Contributed" } as Record<string, string>)[state.kind] || state.kind;
      return <Link key={`${state.kind}:${state.name}`} className="inline-block rounded-xs bg-(--color-bg-secondary) px-2 py-0.5 text-sm leading-5 hover:underline" to={`/imports?record=${state.records[0]}`} title="View source records">
        {label}{state.kind === "owned" && state.quantity > 1 ? ` ×${state.quantity}` : ""}
      </Link>;
    })}
  </div>;
}
