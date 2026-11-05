import React, { useEffect, useState } from "react";
import { average } from "color.js";
import { type ImageList } from "api/api";
import ImageDropHandler from "~/components/ImageDropHandler";
import { Edit, ImageIcon, Plus, EyeOff } from "lucide-react";
import { useAppContext } from "~/providers/AppProvider";
import ImageReplaceModal from "~/components/modals/ImageReplaceModal";
import EditModal from "~/components/modals/EditModal/EditModal";
import AddListenModal from "~/components/modals/AddListenModal";
import MbzIcon from "~/components/icons/MbzIcon";
import { timeListenedString } from "~/utils/utils";
import { Link } from "react-router";
import useWindowWidth from "~/hooks/useWindowWidth";

interface Props {
  filterIssues?: { filter_id: string; name: string; reason: string }[];
  grouped?: boolean;
  type: "Track" | "Album" | "Artist";
  title: string;
  img: ImageList;
  id: number;
  rank: number;
  musicbrainzId: string;
  listenCount: number;
  timeListened: number;
  firstListen: number; // unix
  imgItemId: number;
  children: React.ReactNode;
  subContent?: React.ReactNode;
  metadata?: React.ReactNode;
}

export default function MediaLayout(props: Props) {
  const [bgColor, setBgColor] = useState<string>("(--color-bg)");
  const [imageModalOpen, setImageModalOpen] = useState(false);
  const [renameModalOpen, setRenameModalOpen] = useState(false);
  const [addListenModalOpen, setAddListenModalOpen] = useState(false);
  const { user } = useAppContext();

  useEffect(() => {
    let active = true;
    average(props.img.xs, { amount: 1 })
      .then((color) => {
        if (!active) return;
        setBgColor(`rgba(${color[0]},${color[1]},${color[2]},0.2)`);
      })
      .catch(() => {
        if (active) setBgColor("transparent");
      });
    return () => {
      active = false;
    };
  }, [props.img]);

  const replaceImageCallback = () => {
    window.location.reload();
  };

  const title = `${props.title} - Koito`;

  const mobileIconSize = 22;
  const normalIconSize = 30;

  let vw = useWindowWidth();

  let iconSize = vw > 768 ? normalIconSize : mobileIconSize;

  const mobileTopMarginClass =
    user || props.musicbrainzId != null ? "mt-12" : "mt-4";

  return (
    <main
      className="w-full flex flex-col grow"
      style={{
        background: `linear-gradient(to bottom, ${bgColor}, transparent 800px)`,
      }}
    >
      {!props.grouped && (
        <ImageDropHandler
          itemType={props.type.toLowerCase() === "artist" ? "artist" : "album"}
          onComplete={replaceImageCallback}
        />
      )}
      <title>{title}</title>
      <meta property="og:title" content={title} />
      <meta name="description" content={title} />
      <div
        className={`mx-auto w-19/20 min-w-0 max-w-[1600px] sm:w-[calc(100%-5rem)] ${mobileTopMarginClass} sm:mt-12`}
      >
        <header
          className="flex min-w-0 flex-col items-start gap-6 md:flex-row lg:gap-8"
          aria-label="Music information"
        >
          <div className="aspect-square w-[280px] max-w-full shrink-0 self-start md:w-[320px] xl:w-[360px]">
            <img
              fetchPriority="high"
              src={props.img.large}
              srcSet={`${props.img.large} 1x, ${props.img.xl} 2x`}
              sizes="(min-width: 1280px) 360px, (min-width: 768px) 320px, 280px"
              alt={props.title}
              className="h-full w-full rounded-(--border-radius) border object-cover"
            />
          </div>
          <div className="min-w-0 w-full flex-1 space-y-5">
            <div className="space-y-3">
              <div className="flex min-h-8 items-center justify-between gap-4">
                <p className="flex items-baseline gap-3 text-sm text-(--color-fg-secondary)">
                  <span>{props.type}</span>
                  {props.rank !== 0 && (
                    <span>#{props.rank.toLocaleString()}</span>
                  )}
                </p>
                <div className="flex shrink-0 flex-wrap items-center gap-3">
                  {props.musicbrainzId && (
                    <Link
                      title="View on MusicBrainz"
                      target="_blank"
                      to={`https://musicbrainz.org/${props.type === "Track" ? "recording" : props.type === "Album" ? "release" : "artist"}/${
                        props.musicbrainzId
                      }`}
                    >
                      <MbzIcon size={iconSize} hover />
                    </Link>
                  )}
                  {user && !props.grouped && (
                    <>
                      {props.type === "Track" && (
                        <>
                          <button
                            title="Add Listen"
                            className="hover:cursor-pointer"
                            onClick={() => setAddListenModalOpen(true)}
                          >
                            <Plus
                              size={iconSize}
                              className="hover:stroke-(--color-fg-secondary)"
                            />
                          </button>
                          <AddListenModal
                            open={addListenModalOpen}
                            setOpen={setAddListenModalOpen}
                            trackid={props.id}
                          />
                        </>
                      )}
                      <button
                        title="Edit Item"
                        className="hover:cursor-pointer"
                        onClick={() => setRenameModalOpen(true)}
                      >
                        <Edit
                          size={iconSize}
                          className="hover:stroke-(--color-fg-secondary)"
                        />
                      </button>

                      {props.type !== "Track" && (
                        <button
                          title="Replace Image"
                          className="hover:cursor-pointer"
                          onClick={() => setImageModalOpen(true)}
                        >
                          <ImageIcon
                            size={iconSize}
                            className="hover:stroke-(--color-fg-secondary)"
                          />
                        </button>
                      )}
                      <Link
                        title="Edit inclusion"
                        aria-label="Edit inclusion"
                        className="hover:cursor-pointer"
                        to={`/filters?catalog=${props.type.toLowerCase()}&id=${props.id}&edit=scope`}
                      >
                        <EyeOff
                          size={iconSize}
                          className="hover:stroke-(--color-fg-secondary)"
                        />
                      </Link>
                      <EditModal
                        open={renameModalOpen}
                        setOpen={setRenameModalOpen}
                        type={props.type.toLowerCase()}
                        id={props.id}
                      />
                      <ImageReplaceModal
                        open={imageModalOpen}
                        setOpen={setImageModalOpen}
                        id={props.imgItemId}
                        musicbrainzId={props.musicbrainzId}
                        type={props.type === "Track" ? "Album" : props.type}
                      />
                    </>
                  )}
                </div>
              </div>
              <h1 className="text-3xl font-semibold leading-tight [overflow-wrap:anywhere] sm:text-4xl xl:text-5xl">
                {props.title}
              </h1>
              {props.subContent && (
                <div className="space-y-1.5 text-base text-(--color-fg-secondary) [&_a]:text-(--color-fg)">
                  {props.subContent}
                </div>
              )}
            </div>
            <dl
              className="flex flex-wrap items-baseline gap-x-5 gap-y-2 text-sm text-(--color-fg-secondary)"
              aria-label="Listening statistics"
            >
              <div>
                <dt className="sr-only">Plays</dt>
                <dd>
                  <span className="font-semibold text-(--color-fg)">
                    {props.listenCount.toLocaleString()}
                  </span>{" "}
                  {props.listenCount === 1 ? "play" : "plays"}
                </dd>
              </div>
              {props.timeListened > 0 && (
                <div title={Math.floor(props.timeListened / 3600) + " hours"}>
                  <dt className="sr-only">Time listened</dt>
                  <dd>{timeListenedString(props.timeListened)}</dd>
                </div>
              )}
              {props.firstListen > 0 && (
                <div
                  title={new Date(props.firstListen * 1000).toLocaleString()}
                >
                  <dt className="sr-only">First listen</dt>
                  <dd>
                    since{" "}
                    {new Date(props.firstListen * 1000).toLocaleDateString()}
                  </dd>
                </div>
              )}
            </dl>
            {props.metadata && (
              <div className="min-w-0 space-y-3" aria-label="Music metadata">
                {props.metadata}
              </div>
            )}
          </div>
        </header>
        {!!props.filterIssues?.length && (
          <details className="mt-6 rounded border border-(--color-bg-tertiary) p-3">
            <summary className="cursor-pointer text-sm">
              Unresolved filters
            </summary>
            <ul className="mt-2 space-y-2 text-sm">
              {props.filterIssues.map((issue, index) => (
                <li key={index}>
                  <Link className="underline" to="/filters">
                    {issue.name || "Filter"}
                  </Link>
                  : {issue.reason}
                </li>
              ))}
            </ul>
          </details>
        )}
        {props.children}
      </div>
    </main>
  );
}
