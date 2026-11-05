import { useEffect, useRef, useState } from "react";
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
  const menu = useRef<HTMLDetailsElement>(null);
  useEffect(() => {
    const closeOutside = (event: PointerEvent) => {
      if (!menu.current?.contains(event.target as Node))
        menu.current?.removeAttribute("open");
    };
    const closeEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") menu.current?.removeAttribute("open");
    };
    document.addEventListener("pointerdown", closeOutside);
    document.addEventListener("keydown", closeEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOutside);
      document.removeEventListener("keydown", closeEscape);
    };
  }, []);
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
  if (!states.length) return null;
  const playlists = states.filter((state) => state.kind === "playlist");
  const music = states.filter((state) => state.kind !== "playlist");
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
    <div className="min-w-0 w-full space-y-3" aria-label="Music sources">
      {!!music.length && (
        <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
          {music.map(stateLink)}
        </div>
      )}
      {!!playlists.length && (
        <details ref={menu} className="relative min-w-0 w-full">
          <summary className="w-fit cursor-pointer text-sm underline underline-offset-2">
            Playlists
          </summary>
          <div
            className="absolute left-0 top-full z-20 mt-2 flex max-h-72 w-80 max-w-full flex-wrap items-start gap-2 overflow-y-auto rounded-lg border border-(--color-bg-tertiary) bg-(--color-bg) p-3 shadow-lg"
            aria-label="Playlists"
          >
            {playlists.map(stateLink)}
          </div>
        </details>
      )}
    </div>
  );
}
