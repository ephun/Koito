import { Link } from "react-router";
import type { ReactNode } from "react";

export default function MusicTag({
  to,
  title,
  children,
}: {
  to: string;
  title?: string;
  children: ReactNode;
}) {
  return (
    <Link
      to={to}
      title={title}
      className="inline-flex min-w-0 max-w-full items-center rounded border border-(--color-bg-tertiary) bg-(--color-bg-secondary) px-2 py-0.5 text-sm leading-5 [overflow-wrap:anywhere] hover:underline"
    >
      {children}
    </Link>
  );
}
