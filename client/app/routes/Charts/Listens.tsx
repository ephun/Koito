import ChartLayout from "./ChartLayout";
import { Link, useLoaderData, type LoaderFunctionArgs } from "react-router";
import { type Listen, type PaginatedResponse } from "api/api";
import { timeSince } from "~/utils/utils";
import ArtistLinks from "~/components/ArtistLinks";
import { useAppContext } from "~/providers/AppProvider";
import ListensTable from "~/components/ListensTable";

export async function clientLoader({ request }: LoaderFunctionArgs) {
  const url = new URL(request.url);
  const page = url.searchParams.get("page") || "0";
  url.searchParams.set("page", page);

  const res = await fetch(
    `/apis/web/v1/listens?${url.searchParams.toString()}`,
  );
  if (!res.ok) {
    throw new Response("Failed to load top tracks", { status: 500 });
  }

  const listens: PaginatedResponse<Listen> = await res.json();
  return { listens };
}

export default function Listens() {
  const { listens: initialData } = useLoaderData<{
    listens: PaginatedResponse<Listen>;
  }>();

  return (
    <ChartLayout
      title="Last Played"
      initialData={initialData}
      endpoint="listens"
      render={({ data, page, onNext, onPrev }) => (
        <div className="flex flex-col gap-5 text-sm md:text-[16px] w-11/12 max-w-[1000px]">
          <div className="flex gap-15 mx-auto">
            <button className="default" onClick={onPrev} disabled={page <= 1}>
              Prev
            </button>
            <button
              className="default"
              onClick={onNext}
              disabled={!data.has_next_page}
            >
              Next
            </button>
          </div>
          <div>
            <ListensTable listens={data.items} />
          </div>
          <div className="flex gap-15 mx-auto">
            <button className="default" onClick={onPrev} disabled={page === 0}>
              Prev
            </button>
            <button
              className="default"
              onClick={onNext}
              disabled={!data.has_next_page}
            >
              Next
            </button>
          </div>
        </div>
      )}
    />
  );
}
