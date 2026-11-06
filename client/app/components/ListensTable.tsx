// ListensTable.tsx
import { EyeOff } from "lucide-react";
import ArtistLinks from "./ArtistLinks";
import Image from "./primitives/Image";
import { type Listen, type NowPlaying } from "api/api";
import { Link } from "react-router";
import { timeSince } from "~/utils/utils";
import { useAppContext } from "~/providers/AppProvider";
import FieldHelp from "./FieldHelp";
import { calendarDate } from "~/utils/dates";
import type { ReactNode } from "react";

function CurrentTrackLink({ id, children, className }: { id: number; children: ReactNode; className?: string }) {
  return id > 0 ? <Link className={className} to={`/track/${id}`}>{children}</Link> : <span className={className}>{children}</span>;
}

interface ListensTableProps {
  listens: Listen[];
  npData?: NowPlaying;
  showNP?: boolean;
  hideArtists?: boolean;
  showAlbums?: boolean;
}

export default function ListensTable({
  listens,
  npData,
  showNP,
  hideArtists,
  showAlbums,
}: ListensTableProps) {
  const { user } = useAppContext();
  const imgColSizeClasses = "py-3 min-w-8 sm:min-w-11";
  const imgSize = 32;
  const timeColClasses = "text-(--color-fg-tertiary) pr-2 sm:pr-4 sm:text-sm";

  return (
    <table className="table border-collapse mt-6 w-full">
      <tbody>
        {showNP && npData && (
          <tr className="group border-b border-(--color-bg-tertiary) relative last:border-b-0">
            <td className={imgColSizeClasses}>
              <CurrentTrackLink id={npData.track.id}>
                <Image
                  src={npData.track.image.small}
                  size={imgSize}
                  alt={npData.track.title}
                  lazy
                />
              </CurrentTrackLink>
            </td>
            <td className="max-w-0 w-full px-2 py-2 align-middle">
              <div>
                {!hideArtists && (
                  <>
                    <ArtistLinks artists={npData.track.artists} />
                    {" — "}
                  </>
                )}
                <CurrentTrackLink
                  className="hover:text-(--color-fg-secondary)"
                  id={npData.track.id}
                >
                  {npData.track.title}
                </CurrentTrackLink>
              </div>
            </td>
            <td className={timeColClasses}>
              <div className="flex items-center justify-end gap-2 text-end">
                <div className="h-1.5 w-1.5 rounded-full bg-(--color-primary)" />
                {"Now Playing"}
              </div>
            </td>
          </tr>
        )}
        {listens.map((item) => (
          <tr
            key={`last_listen_${item.track.id}_${item.time}_${item.play_key || ""}`}
            className="group border-b border-(--color-bg-tertiary) relative last:border-b-0"
          >
            <td className={imgColSizeClasses}>
              <Link to={`/track/${item.track.id}`}>
                <Image
                  src={item.track.image.small}
                  size={imgSize}
                  alt={item.track.title}
                />
              </Link>
            </td>
            <td className="max-w-0 w-full px-2 py-2">
              <div>
                {!hideArtists && (
                  <>
                    <ArtistLinks artists={item.track.artists} />
                    {" — "}
                  </>
                )}
                <Link
                  className="hover:text-(--color-fg-secondary)"
                  to={`/track/${item.track.id}`}
                >
                  {item.track.title}
                </Link>
                {item.evidence === "estimated" && <span className="ml-2 inline-flex items-center gap-1 text-xs text-(--color-fg-tertiary)">Estimated<FieldHelp label="Estimated play">Estimated from a library count and its supported dates.</FieldHelp></span>}
                {item.evidence === "last_played" && <span className="ml-2 inline-flex items-center gap-1 text-xs text-(--color-fg-tertiary)">Last played<FieldHelp label="Last-played evidence">Reported as the library's most recent play.</FieldHelp></span>}
                {item.attribution_basis === "heuristic" && <span className="ml-2 inline-flex items-center gap-1 text-xs text-(--color-fg-tertiary)">Heuristic<FieldHelp label="Heuristic attribution">Assigned to you using the enabled heuristic.</FieldHelp></span>}
              </div>
              {!!item.tags?.length && <ul aria-label="Listening tags" className="mt-1 flex min-w-0 max-w-full flex-wrap items-center gap-1">{item.tags.map((tag,index) => <li key={index} className="inline-flex min-w-0 max-w-full items-center [overflow-wrap:anywhere] rounded border border-(--color-bg-tertiary) px-2 py-0.5 text-xs leading-4 text-(--color-fg-secondary)">{tag}</li>)}</ul>}
              {showAlbums && item.album_id && <Link className="mt-1 block truncate text-xs text-(--color-fg-secondary) hover:underline" to={`/album/${item.album_id}`}>{item.album}</Link>}
            </td>
            <td
              className={`text-end whitespace-nowrap ${timeColClasses}`}
              title={item.date_only && item.local_date ? calendarDate(item.local_date) : new Date(item.time).toString()}
            >
              <span className="inline-flex items-center justify-end gap-2">
              {user?.role === "admin" && item.record_id ? <Link className="hover:underline underline-offset-2" title="View records" to={`/imports?record=${item.record_id}`}>{item.date_only && item.local_date ? calendarDate(item.local_date) : timeSince(new Date(item.time))}</Link> : item.date_only && item.local_date ? calendarDate(item.local_date) : timeSince(new Date(item.time))}
              {item.time_estimated && <FieldHelp label="Estimated time">This time comes from an enabled Time filter.</FieldHelp>}
              {user?.role === "admin" && <Link aria-label="Edit play inclusion" title="Edit play inclusion" to={`/filters?listen_track=${item.track.id}&listen_time=${encodeURIComponent(item.time)}&listen_key=${encodeURIComponent(item.play_key || "")}${item.record_id && (!item.evidence || item.evidence === "observed") ? `&play_record=${item.record_id}` : ""}`} className="inline-flex h-6 w-6 shrink-0 items-center justify-center sm:opacity-0 sm:group-hover:opacity-100 focus:opacity-100"><EyeOff size={14} /></Link>}
              </span>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
