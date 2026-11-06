import { Link, useLoaderData, type LoaderFunctionArgs } from "react-router";
import { type Track } from "api/api";
import LastPlayed from "~/components/LastPlayed";
import MediaLayout from "./MediaLayout";
import ActivityGrid from "~/components/ActivityGrid";
import { timeListenedString } from "~/utils/utils";
import InterestGraph from "~/components/InterestGraph";
import MusicStates from "~/components/MusicStates";

export async function clientLoader({ params }: LoaderFunctionArgs) {
  const res = await fetch(`/apis/web/v1/track/${params.id}`);
  if (!res.ok) {
    throw new Response("Failed to load track", { status: res.status });
  }
  const track: Track = await res.json();
  return { track };
}

export default function Track() {
  const { track } = useLoaderData<typeof clientLoader>();

  return (
    <MediaLayout
      filterIssues={[...(track.name_issues || []), ...(track.identifier_issues || []), ...(track.scope_issues || [])]}
      type="Track"
      title={track.title}
      img={track.image}
      id={track.id}
      rank={track.all_time_rank}
      musicbrainzId={track.musicbrainz_id}
      timeListened={track.time_listened}
      listenCount={track.listen_count}
      firstListen={track.first_listen}
      firstListenDate={track.first_listen_date}
      imgItemId={track.album_id}
      metadata={<MusicStates kind="track" id={track.id} />}
      subContent={
        <>
          {track.artists.length > 0 && (
            <p>
              by{" "}
              {track.artists.map((artist, i) => (
                <span key={artist.id}>
                  {i > 0 && i !== track.artists.length - 1 && ", "}
                  {i === track.artists.length - 1 && track.artists.length > 1
                    ? " and "
                    : ""}
                  <Link className="hover:underline" to={`/artist/${artist.id}`}>
                    {artist.name}
                  </Link>
                </span>
              ))}
            </p>
          )}
          <p>
            Appears on{" "}
            {track.albums?.map((album, index) => <span key={album.id}>
              {index > 0 && ", "}
              <Link className="hover:underline" to={`/album/${album.id}`} title={`${album.listen_count.toLocaleString()} plays`}>
                {album.title}
              </Link>
            </span>)}
          </p>
        </>
      }
    >
      {track.first_listen > 0 && <div className="flex flex-col gap-10 md:gap-20 mt-14">
        <div className="min-w-0 flex-1">
          <LastPlayed limit={11} trackId={track.id} musicTrackId={track.music_track_id} showAlbums showNowPlaying showSeeMore />
        </div>
        <div className="flex flex-wrap gap-10">
          <InterestGraph type="track" id={track.id} />
          <ActivityGrid configurable trackId={track.id} />
        </div>
      </div>}
    </MediaLayout>
  );
}
