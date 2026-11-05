import { Link, useLoaderData, type LoaderFunctionArgs } from "react-router";
import TopTracks from "~/components/TopTracks";
import { type Artist } from "api/api";
import LastPlayed from "~/components/LastPlayed";
import MediaLayout from "./MediaLayout";
import ArtistAlbums from "~/components/ArtistAlbums";
import ActivityGrid from "~/components/ActivityGrid";
import { timeListenedString } from "~/utils/utils";
import InterestGraph from "~/components/InterestGraph";
import MediaItemNote from "~/components/MediaItemNote";
import { useAppContext } from "~/providers/AppProvider";

export async function clientLoader({ params }: LoaderFunctionArgs) {
  const res = await fetch(`/apis/web/v1/artist/${params.id}`);
  if (!res.ok) {
    throw new Response("Failed to load artist", { status: 500 });
  }
  const artist: Artist = await res.json();
  return artist;
}

export default function Artist() {
  const artist = useLoaderData() as Artist;
  const period = "all_time";
  const {user} = useAppContext();

  let index = artist.aliases.indexOf(artist.name);
  if (index !== -1) {
    artist.aliases.splice(index, 1);
  }

  return (
    <MediaLayout
      filterIssues={[...(artist.name_issues || []), ...(artist.identifier_issues || [])]}
      type="Artist"
      title={artist.name}
      img={artist.image}
      id={artist.id}
      rank={artist.all_time_rank}
      musicbrainzId={artist.musicbrainz_id}
      timeListened={artist.time_listened}
      listenCount={artist.listen_count}
      firstListen={artist.first_listen}
      imgItemId={artist.id}
      subContent={<div className="flex max-w-full flex-wrap items-center gap-x-2 gap-y-1">{artist.genres?.map(genre => <Link key={genre} className="inline-flex max-w-full items-center rounded border border-(--color-bg-tertiary) px-2 py-0.5 text-sm leading-5 [overflow-wrap:anywhere] hover:underline" to={`/chart/top/artists?period=all_time&genre=${encodeURIComponent(genre)}`}>{genre}</Link>)}{user?.role === "admin" && <Link className="inline-flex shrink-0 items-center py-0.5 text-sm leading-5 underline underline-offset-2" to={`/filters?artist=${artist.id}`}>Edit genres</Link>}</div>}
    >
      {!!artist.genre_issues?.length && <details className="mt-6 rounded border border-(--color-bg-tertiary) p-3">
        <summary className="cursor-pointer text-sm">Unresolved genre filters</summary>
        <ul className="mt-2 space-y-2 text-sm">{artist.genre_issues.map((issue,index) => <li key={index}><Link className="underline" to="/filters">{issue.name || "Filter"}</Link>: {issue.reason}</li>)}</ul>
      </details>}
      <div className="flex flex-col gap-14">
        <div className="flex flex-col gap-10 md:gap-12 mt-8 max-w-[1400px]">
          <div className="flex gap-10 md:gap-20 flex-wrap xl:flex-nowrap items-start">
            <TopTracks
              limit={8}
              period={period}
              artistId={artist.id}
              showSeeMore
              className="max-w-[750px] lg:max-w-[450px]"
            />
            <div className="min-w-0 w-full sm:min-w-[350px] flex-1">
              <LastPlayed
                limit={11}
                artistId={artist.id}
                showNowPlaying
                showSeeMore
              />
            </div>
          </div>
          <div className="flex flex-wrap gap-10">
            <InterestGraph type="artist" id={artist.id} />
            <ActivityGrid configurable artistId={artist.id} />
          </div>
        </div>
        <ArtistAlbums
          artistId={artist.id}
          name={artist.name}
          period="all_time"
        />
      </div>
    </MediaLayout>
  );
}
