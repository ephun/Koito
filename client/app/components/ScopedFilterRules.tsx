import { useId, useState } from "react";
import { defaultRuleSettings, sourceName, type FilterState, type Rule, type RuleScope } from "~/utils/filterRules";
import { Option, Switch, filterInput as input, filterButton as button } from "./FilterControls";
import FieldHelp from "./FieldHelp";
import { X } from "lucide-react";

export function libraryRule(source: FilterState["sources"][number], subtype: "count" | "time"): Rule {
  return { id: `itunes-${subtype}-${source.id}`, type: "inference", subtype, enabled: false,
    scope: { service: "itunes", source_id: source.id },
    settings: { ...defaultRuleSettings, ...(subtype === "count" ? { count_mode: "maximum" } : { use_last_played: false, estimate_times: false }) } };
}

export function ScopedRule({ rule, state, busy, blocked = false, onChange, onRemove }: {
  rule: Rule; state: FilterState; busy: boolean; blocked?: boolean;
  onChange: (rule: Rule) => void; onRemove?: () => void;
}) {
  const id = useId();
  const disabled = busy || !rule.enabled || blocked;
  const set = (settings: Partial<Rule["settings"]>) => onChange({ ...rule, settings: { ...rule.settings, ...settings } });
  const source = state.sources.find(source => source.id === rule.scope.source_id);
  const service = rule.scope.service || source?.service || "";
  const name = ruleName(rule, state);
  const settings = rule.settings;
  const title = rule.type === "inference" ? rule.subtype === "count" ? "Count" : "Time" : subtypeName(rule.subtype);
  const toggleLabel = `${title} for ${name}`;

  return <section aria-label={toggleLabel} className="space-y-4 p-4 sm:p-5">
    <div className="flex items-start justify-between gap-3">
      <div className="min-w-0"><div className="flex min-w-0 items-center gap-1"><h4 className="min-w-0 break-words font-medium">{name}</h4>
        {blocked && <FieldHelp label={`${name} timing`}>Enable Count to use library timing.</FieldHelp>}
      </div>
        {conditions.filter(condition => !["file_hash", "library_id", "record_id"].includes(condition.key) && rule.scope[condition.key]).map(condition => <p key={condition.key} className="break-words text-xs text-(--color-fg-secondary)">{condition.label}: {rule.scope[condition.key]}</p>)}
      </div>
      <Switch label={toggleLabel} checked={rule.enabled} disabled={busy || blocked} onChange={enabled => onChange({ ...rule, enabled })} />
    </div>
    <div className={`${disabled ? "opacity-50" : ""} space-y-4`}>
      {rule.type === "inference" && rule.subtype === "count" && <Option label="Cumulative counts" id={`${id}-count`} help="Choose how iTunes library export records contribute to totals.">
        <select id={`${id}-count`} className={input} value={settings.count_mode || "maximum"} disabled={disabled} onChange={event => set({ count_mode: event.target.value })}>
          <option value="maximum">Largest reported count</option><option value="latest">Latest reported count</option><option value="reset_total">Assume decreases restart the counter</option>
        </select>
      </Option>}
      {rule.type === "inference" && rule.subtype === "time" && <>
        <Option label="Include last known play" id={`${id}-last`} help="Use the last-played date recorded in the iTunes library export."><Switch id={`${id}-last`} label={`Use last played for ${name}`} checked={!!settings.use_last_played} disabled={disabled} onChange={use_last_played => set({ use_last_played })} /></Option>
        <Option label="Estimate other play dates" id={`${id}-estimate`} help="Estimate missing play dates within the time range supported by the iTunes library export."><Switch id={`${id}-estimate`} label={`Estimate remaining times for ${name}`} checked={!!settings.estimate_times} disabled={disabled} onChange={estimate_times => set({ estimate_times })} /></Option>
      </>}
      {rule.type === "attribution" && rule.subtype === "direct" && <Option label="Listener" id={`${id}-listener`} help="Assign listening within this scope.">
        <select id={`${id}-listener`} className={input} value={settings.listener || "unknown"} disabled={disabled} onChange={event => set({ listener: event.target.value })}><option value="self">Me</option><option value="other">Someone else</option><option value="unknown">Unknown</option></select>
      </Option>}
      {rule.subtype === "heuristic" && <>
        <Option label="Evidence source" id={`${id}-evidence`} help="Use matching plays assigned to you by Direct rules.">
          <select id={`${id}-evidence`} className={input} value={settings.evidence_source_id || ""} disabled={disabled} onChange={event => set({ evidence_source_id: Number(event.target.value) })}>
            <option value="" disabled>Choose source</option>{state.sources.filter(source => source.records > 0 && source.service !== "itunes").map(source => <option key={source.id} value={source.id}>{sourceName(source.service)}</option>)}
          </select>
        </Option>
        <Option label="Minimum matches" id={`${id}-minimum`} help="Require this many distinct personal plays of matching music."><input id={`${id}-minimum`} type="number" min="1" max="100000" step="1" className={`${input} w-28`} disabled={disabled} value={settings.minimum_matches || 1} onChange={event => set({ minimum_matches: Number(event.target.value) })} /></Option>
      </>}
      {(rule.subtype === "aggregate" || rule.subtype === "heuristic" && service === "itunes") && <Option label="My share" id={`${id}-share`} help="Assign this percentage of the count to you. Whole plays round down.">
        <span className="flex items-center gap-2"><input id={`${id}-share`} type="number" min="0" max="100" step="1" className={`${input} w-24`} disabled={disabled} value={settings.share_percent ?? 0} onChange={event => set({ share_percent: Number(event.target.value) })} /><span>%</span></span>
      </Option>}
      <ScopeEditor rule={rule} state={state} disabled={disabled} onChange={scope => onChange({ ...rule, scope })} />
    </div>
    {onRemove && <button type="button" className="text-sm text-(--color-fg-secondary) underline disabled:opacity-50" disabled={busy} onClick={onRemove}>Remove rule</button>}
  </section>;
}

export function AddScopedRule({ state, type, subtype, busy, onAdd }: { state: FilterState; type: string; subtype?: string; busy: boolean; onAdd: (rule: Rule) => void }) {
  const [open, setOpen] = useState(false);
  const [choice, setChoice] = useState(subtype || "direct");
  const [sourceID, setSourceID] = useState("");
  const id = useId();
  const sources = state.sources.filter(source => source.records > 0 && (type === "attribution" && choice !== "aggregate" || source.service === "itunes"));
  const selected = sources.find(source => String(source.id) === sourceID) || sources[0];
  const observed = state.sources.filter(source => source.records > 0 && source.service !== "itunes");

  function add() {
    if (!selected) return;
    const rule: Rule = { id: crypto.randomUUID(), type, subtype: choice, enabled: false,
      scope: { service: selected.service, source_id: selected.id }, settings: { ...defaultRuleSettings } };
    if (type === "inference") Object.assign(rule.settings, choice === "count" ? { count_mode: "maximum" } : { use_last_played: false, estimate_times: false });
    if (choice === "direct") rule.settings.listener = "unknown";
    if (choice === "aggregate" || choice === "heuristic" && selected.service === "itunes") rule.settings.share_percent = 0;
    if (choice === "heuristic") Object.assign(rule.settings, { minimum_matches: 1, evidence_source_id: observed[0]?.id });
    onAdd(rule); setOpen(false);
  }

  if (!open) return <button type="button" className="m-4 text-sm underline disabled:opacity-50 sm:m-5" disabled={busy} onClick={() => setOpen(true)}>Add rule</button>;
  return <div aria-label={`Add ${type} rule`} className="space-y-3 p-4 sm:p-5">
    {type === "attribution" && <div className="flex flex-wrap items-center justify-between gap-2 text-sm"><label htmlFor={`${id}-type`}>Type</label><select id={`${id}-type`} className={input} disabled={busy} value={choice} onChange={event => setChoice(event.target.value)}>
      <option value="direct">Direct</option>{observed.length > 0 && <option value="heuristic">Heuristic</option>}{state.sources.some(source => source.service === "itunes" && source.records > 0) && <option value="aggregate">Aggregate</option>}
    </select></div>}
    <div className="flex flex-wrap items-center justify-between gap-2 text-sm"><label htmlFor={`${id}-source`}>Source</label><select id={`${id}-source`} className={input} value={selected?.id || ""} disabled={busy} onChange={event => setSourceID(event.target.value)}>{sources.map(source => <option key={source.id} value={source.id}>{sourceName(source.service)}</option>)}</select></div>
    <div className="flex gap-3"><button type="button" className={button} disabled={busy || !selected} onClick={add}>Add</button><button type="button" className="px-2 underline" disabled={busy} onClick={() => setOpen(false)}>Cancel</button></div>
  </div>;
}

const conditions: { key: keyof RuleScope; label: string; help: string }[] = [
  { key: "file_hash", label: "File", help: "Apply the rule to records from this file." },
  { key: "library_id", label: "Library", help: "Apply the rule to this library and corroborated converted copies." },
  { key: "track_id", label: "Persistent track ID", help: "Use the track identity supplied by the library." },
  { key: "record_id", label: "Record", help: "Apply the rule to one preserved record." },
  { key: "artist", label: "Artist", help: "Match the exact artist field supplied by the source." },
  { key: "device", label: "Device", help: "Match the supplied device name or Spotify platform." },
  { key: "from", label: "From", help: "Include listening from this ISO timestamp. Include a timezone." },
  { key: "until", label: "Until", help: "Include listening before this ISO timestamp. Include a timezone." },
];

function ScopeEditor({ rule, state, disabled, onChange }: { rule: Rule; state: FilterState; disabled: boolean; onChange: (scope: RuleScope) => void }) {
  const id = useId();
  const libraryOnly = rule.type === "inference" || rule.subtype === "aggregate";
  const noDates = rule.subtype === "count" || rule.subtype === "aggregate";
  const available = conditions.filter(condition => (!noDates || !["from", "until", "device"].includes(condition.key)) && (rule.scope.service === "itunes" || !["library_id", "track_id"].includes(condition.key)));
  const sources = state.sources.filter(source => source.records > 0 && (!libraryOnly || source.service === "itunes"));
  const fields = available.filter(condition => rule.scope[condition.key] !== undefined);
  function set(key: keyof RuleScope, value: string | number) { onChange({ ...rule.scope, [key]: value }); }
  return <details>
    <summary className="cursor-pointer text-sm text-(--color-fg-secondary)">Scope</summary>
    <div className="mt-4 space-y-4">
      <Option label="Source" id={`${id}-source`} help="Choose which imported source this rule interprets.">
        <select id={`${id}-source`} className={input} disabled={disabled} value={rule.scope.source_id || `service:${rule.scope.service || ""}`} onChange={event => {
          const source = sources.find(source => source.id === Number(event.target.value));
          if (source) onChange({ service: source.service, source_id: source.id });
        }}>
          {!rule.scope.source_id && <option value={`service:${rule.scope.service || ""}`}>{sourceName(rule.scope.service || "All sources")}</option>}
          {sources.map(source => <option key={source.id} value={source.id}>{sourceName(source.service)}</option>)}
        </select>
      </Option>
      {fields.map(condition => <div className="space-y-1" key={condition.key}>
        <Option label={condition.label} id={`${id}-${condition.key}`} help={condition.help}>
          <div className="flex min-w-0 gap-2">
            {condition.key === "file_hash" ? <select id={`${id}-${condition.key}`} className={input} disabled={disabled} value={String(rule.scope.file_hash || "")} onChange={event => set("file_hash", event.target.value)}>
              <option value="">Choose file</option>{state.files.filter(file => (!rule.scope.source_id || file.source_id === rule.scope.source_id) && (!rule.scope.service || file.service === rule.scope.service)).map(file => <option key={file.hash} value={file.hash}>{file.filename}</option>)}
            </select> : condition.key === "library_id" ? <select id={`${id}-${condition.key}`} className={input} disabled={disabled} value={String(rule.scope.library_id || "")} onChange={event => set("library_id", event.target.value)}>
              <option value="">Choose library</option>{state.libraries.filter(library => !rule.scope.source_id || library.source_id === rule.scope.source_id).map(library => <option key={library.id} value={library.id}>{library.filename}</option>)}
            </select> : <input id={`${id}-${condition.key}`} className={`${input} w-full sm:w-60`} disabled={disabled} type={condition.key === "record_id" ? "number" : "text"} min={condition.key === "record_id" ? 1 : undefined} value={rule.scope[condition.key] || ""} placeholder={["from", "until"].includes(condition.key) ? "2020-01-01T00:00:00Z" : undefined} onChange={event => set(condition.key, condition.key === "record_id" ? Number(event.target.value) : event.target.value)} />}
            <button type="button" aria-label={`Remove ${condition.label} condition`} className="shrink-0 px-2 text-sm disabled:opacity-50" disabled={disabled} onClick={() => { const scope = { ...rule.scope }; delete scope[condition.key]; onChange(scope); }}><X size={14} aria-hidden="true" /></button>
          </div>
        </Option>
      </div>)}
      {available.some(condition => rule.scope[condition.key] === undefined) && <select className={`${input} text-sm`} aria-label="Add scope condition" disabled={disabled} value="" onChange={event => { if (event.target.value) set(event.target.value as keyof RuleScope, event.target.value === "record_id" ? 0 : ""); }}><option value="">Add condition</option>{available.filter(condition => rule.scope[condition.key] === undefined).map(condition => <option key={condition.key} value={condition.key}>{condition.label}</option>)}</select>}
    </div>
  </details>;
}

export function subtypeName(subtype: string) { return ({ direct: "Direct", heuristic: "Heuristic", aggregate: "Aggregate" } as Record<string, string>)[subtype] || subtype; }

function ruleName(rule: Rule, state: FilterState) {
  if (rule.scope.record_id) return `Record #${rule.scope.record_id}`;
  if (rule.scope.file_hash) return state.files.find(file => file.hash === rule.scope.file_hash)?.filename || "Selected file";
  if (rule.scope.library_id) return state.libraries.find(library => library.id === rule.scope.library_id)?.filename || "Selected library";
  return sourceName(rule.scope.service || state.sources.find(source => source.id === rule.scope.source_id)?.service || "All sources");
}
