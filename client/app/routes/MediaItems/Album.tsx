import { Link, useLoaderData, type LoaderFunctionArgs } from "react-router";
import TopTracks from "~/components/TopTracks";
import { type Album } from "api/api";
import LastPlayed from "~/components/LastPlayed";
import MediaLayout from "./MediaLayout";
import ActivityGrid from "~/components/ActivityGrid";
import InterestGraph from "~/components/InterestGraph";
import MusicStates from "~/components/MusicStates";

export async function clientLoader({ params, request }: LoaderFunctionArgs) {
  const res = await fetch(`/apis/web/v1/album/${params.id}`, { signal: request.signal });
  if (!res.ok) {
    throw new Response("Failed to load album", { status: 500 });
  }
  const album: Album = await res.json();
  return album;
}

export default function Album() {
  const album = useLoaderData() as Album;
  const period = "all_time";


  return (
    <MediaLayout
      filterIssues={[...(album.name_issues || []), ...(album.identifier_issues || []), ...(album.classification_issues || [])]}
      grouped={!!album.album_group_id}
      type="Album"
      title={album.title}
      img={album.image}
      id={album.id}
      rank={album.all_time_rank}
      musicbrainzId={album.musicbrainz_id}
      timeListened={album.time_listened}
      listenCount={album.listen_count}
      firstListen={album.first_listen}
      firstListenDate={album.first_listen_date}
      imgItemId={album.id}
      metadata={<MusicStates kind="album" id={album.id} />}
      subContent={
        <>
          {album.artists.length > 0 && !album.is_various_artists && (
            <p>
              by{" "}
              {
                <span key={album.artists[0].id}>
                  <Link
                    className="hover:underline"
                    to={`/artist/${album.artists[0].id}`}
                  >
                    {album.artists[0].name}
                  </Link>
                </span>
              }
            </p>
          )}
          {album.is_various_artists && <p>by Various Artists</p>}
        </>
      }
    >
      {album.editions && <section className="mt-8 w-full max-w-3xl" aria-labelledby="editions-title">
        <h2 id="editions-title" className="mb-3">Editions</h2>
        <ul className="space-y-2">{album.editions.map(edition => <li key={edition.id} className="flex items-center justify-between gap-4">
          <Link className="min-w-0 hover:underline" to={`/album/${edition.id}`}>{edition.title}</Link>
          <span className="shrink-0 text-sm text-(--color-fg-secondary)">{edition.listen_count} plays</span>
        </li>)}</ul>
      </section>}
      {album.listen_count > 0 && <div className="flex flex-col gap-10 md:gap-12 mt-8 max-w-[1400px]">
        <div className="flex gap-10 md:gap-20 flex-wrap xl:flex-nowrap items-start">
          <TopTracks
            limit={8}
            period={period}
            albumId={album.album_group_id ? undefined : album.id}
            albumGroupId={album.album_group_id}
            showSeeMore
            className="max-w-[750px] lg:max-w-[450px]"
          />
          {album.first_listen > 0 && <div className="min-w-[350px] flex-1">
            <LastPlayed
              limit={11}
              albumId={album.album_group_id ? undefined : album.id}
              albumGroupId={album.album_group_id}
              showNowPlaying={!album.album_group_id}
              showAlbums={!!album.album_group_id}
              showSeeMore
            />
          </div>}
        </div>
        {album.first_listen > 0 && <div className="flex flex-wrap gap-10">
          <InterestGraph type={album.album_group_id ? "album-group" : "album"} id={album.album_group_id || album.id} />
          <ActivityGrid configurable albumId={album.album_group_id ? undefined : album.id} albumGroupId={album.album_group_id} />
        </div>}
      </div>}
    </MediaLayout>
  );
}
