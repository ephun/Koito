import { useEffect, useState } from "react";
import { useAppContext } from "~/providers/AppProvider";
import MusicTag from "./MusicTag";

type MusicState = {
  kind: string;
  name: string;
  records: number[];
  quantity: number;
};

export default function MusicStates({
  kind,
  id,
}: {
  kind: "track" | "album";
  id: number;
}) {
  const { user } = useAppContext();
  const [states, setStates] = useState<MusicState[]>([]);
  useEffect(() => {
    setStates([]);
    if (user?.role !== "admin") return;
    const controller = new AbortController();
    fetch(`/apis/web/v1/imports/music?${kind}=${id}`, {
      signal: controller.signal,
    })
      .then((response) => {
        if (!response.ok) throw new Error("Could not load music sources.");
        return response.json() as Promise<MusicState[]>;
      })
      .then(setStates)
      .catch((error) => {
        if (error.name !== "AbortError") console.error(error);
      });
    return () => controller.abort();
  }, [kind, id, user?.role]);
  const displayedStates = states.filter((state) => state.kind !== "playlist");
  if (!displayedStates.length) return null;
  const stateLink = (state: MusicState) => {
    const label =
      state.kind === "playlist"
        ? state.name
        : (
            {
              saved_tracks: kind === "album" ? "Saved tracks" : "Saved",
              saved_albums: "Saved album",
              catalog: "Navidrome library",
              owned: "Owned",
              wanted: "Wanted",
              inventory: "Inventory",
              contributions: "Contributed",
            } as Record<string, string>
          )[state.kind] || state.kind;
    return (
      <MusicTag
        key={`${state.kind}:${state.name}`}
        to={`/imports?record=${state.records[0]}`}
        title="View source records"
      >
        {label}
        {state.kind === "owned" && state.quantity > 1
          ? ` ×${state.quantity}`
          : ""}
      </MusicTag>
    );
  };
  return (
    <div className="flex min-w-0 max-w-full flex-wrap items-center gap-2" aria-label="Music sources">
      {displayedStates.map(stateLink)}
    </div>
  );
}
