import { useId, useRef, useState } from "react";
import { CircleHelp } from "lucide-react";

export default function FieldHelp({ label, children }: { label: string; children: string }) {
  const id = useId();
  const anchor = useRef<HTMLSpanElement>(null);
  const [open, setOpen] = useState(false);
  const [left, setLeft] = useState(0);
  const [below, setBelow] = useState(false);
  function show() {
    const box = anchor.current?.getBoundingClientRect();
    if (box) {
      const width = Math.min(208, window.innerWidth - 32);
      const position = Math.max(16, Math.min(box.left + (box.width - width) / 2, window.innerWidth - width - 16));
      setLeft(position - box.left);
      setBelow(box.top < 112);
    }
    setOpen(true);
  }
  return <span
    ref={anchor}
    className="relative inline-flex shrink-0"
    onMouseEnter={show}
    onMouseLeave={() => setOpen(false)}
    onFocus={show}
    onBlur={() => setOpen(false)}
    onKeyDown={event => { if (event.key === "Escape") { setOpen(false); event.stopPropagation(); } }}
  >
    <button
      type="button"
      aria-label={`About ${label}`}
      aria-describedby={id}
      className="inline-flex h-6 w-6 items-center justify-center rounded text-(--color-fg-tertiary) hover:text-(--color-fg)"
      onClick={show}
    ><CircleHelp size={15} aria-hidden="true" /></button>
    <span
      id={id}
      role="tooltip"
      style={{ left }}
      className={`${open ? "block" : "hidden"} ${below ? "top-full pt-2" : "bottom-full pb-2"} absolute z-50 w-52 max-w-[calc(100vw-2rem)] text-left text-sm font-normal normal-case tracking-normal`}
    ><span className="block rounded border border-(--color-bg-tertiary) bg-(--color-bg) p-3 text-(--color-fg) shadow-lg">{children}</span></span>
  </span>;
}
