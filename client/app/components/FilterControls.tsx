import type { ReactNode } from "react";
import FieldHelp from "./FieldHelp";

export const filterButton = "rounded border border-(--color-bg-tertiary) px-4 py-2 hover:bg-(--color-bg-tertiary) disabled:opacity-50 disabled:cursor-default";
export const filterInput = "min-w-0 max-w-full rounded border border-(--color-bg-tertiary) bg-(--color-bg) px-3 py-2 disabled:cursor-default";
export const filterCard = "rounded-lg border border-(--color-bg-tertiary) bg-(--color-bg-secondary) divide-y divide-(--color-bg-tertiary)";

export function Option({ label, id, help, children }: { label: string; id?: string; help: string; children: ReactNode }) {
  return <div className="flex flex-col justify-between gap-2 sm:flex-row sm:items-center sm:gap-6">
    <span className="inline-flex items-center gap-1 text-sm"><label htmlFor={id}>{label}</label><FieldHelp label={label}>{help}</FieldHelp></span>
    <div className="flex min-w-0 max-w-full items-center sm:justify-end">{children}</div>
  </div>;
}

export function Switch({ id, label, checked, disabled, onChange }: { id?: string; label: string; checked: boolean; disabled?: boolean; onChange: (checked: boolean) => void }) {
  return <button id={id} type="button" role="switch" aria-label={label} aria-checked={checked} disabled={disabled} onClick={() => onChange(!checked)} className="inline-flex shrink-0 items-center gap-2 rounded px-1 py-1 disabled:cursor-default disabled:opacity-50">
    <span aria-hidden="true" className="w-6 text-left text-xs text-(--color-fg-secondary)">{checked ? "On" : "Off"}</span>
    <span aria-hidden="true" className={`${checked ? "bg-(--color-primary)" : "bg-(--color-bg-tertiary)"} relative h-5 w-9 rounded-full transition-colors`}><span className={`${checked ? "translate-x-4" : "translate-x-0"} absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-(--color-bg) shadow-sm transition-transform`} /></span>
  </button>;
}
