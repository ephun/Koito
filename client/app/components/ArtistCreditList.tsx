import { useQuery } from "@tanstack/react-query";
import ComboBox from "./ComboBox";
import type { Artist } from "api/api";

export default function ArtistCreditList({ ids, disabled, onChange }: {
  ids: number[];
  disabled: boolean;
  onChange: (ids: number[]) => void;
}) {
  const names = useQuery({
    queryKey: ["credit-artists", ids],
    queryFn: async ({ signal }) => Promise.all(ids.map(async id => {
      const response = await fetch(`/apis/web/v1/artist/${id}`, { signal });
      if (!response.ok) throw new Error("Could not load artist credits.");
      return response.json() as Promise<Artist>;
    })),
  });
  return <div className="min-w-0 space-y-2">
    {ids.map(id => {
      const name = names.data?.find(artist => artist.id === id)?.name || "Loading…";
      return <div key={id} className="flex min-w-0 items-center justify-between gap-3">
        <span className="break-words">{name}</span>
        <button type="button" disabled={disabled} className="shrink-0 text-sm underline" aria-label={`Remove ${name}`} onClick={() => onChange(ids.filter(existing => existing !== id))}>Remove</button>
      </div>;
    })}
    {names.error && <p role="alert">{names.error.message}</p>}
    {!disabled && <ComboBox filterFunction={result => ({ artists: result.artists.filter(artist => !ids.includes(artist.id)), albums: [], tracks: [] })} onSelection={(artist: Artist) => onChange([...ids, artist.id])} />}
  </div>;
}
