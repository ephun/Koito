import { useState } from "react";
import FilterBlockEditor, { expressionSummary, statementSummary } from "./FilterBlockEditor";
import FieldHelp from "./FieldHelp";
import { Switch, filterInput as input, filterButton as button } from "./FilterControls";
import { defaultRuleSettings, filterRequest, type FilterState, type Rule, type RuleConfig } from "~/utils/filterRules";

type NamingExample = { record_id: number; field: string; before: string; after: string };

export default function ProgramFilterManager({ state, config, busy, type = "presentation", subtype = "naming", embedded = false, onChange, onRemove }: {
  state: FilterState; config: RuleConfig; busy: boolean; type?: string; subtype?: string; embedded?: boolean; onChange: (rule: Rule) => void; onRemove: (id: string) => void;
}) {
  const [expanded, setExpanded] = useState<string>();
  const [checking, setChecking] = useState(false);
  const [examples, setExamples] = useState<NamingExample[]>();
  const [checkedConfig, setCheckedConfig] = useState("");
  const [checkedVersion, setCheckedVersion] = useState(0);
  const [error, setError] = useState("");
  const schema = state.blocks?.schemas[`${type}:${subtype}`];
  if (!schema || !state.blocks) return null;
  const rules = config.rules.filter(rule => rule.type === type && rule.subtype === subtype);
  const title = subtype === "direct" ? "Direct attribution" : "Naming";
  const current = rules.find(rule => rule.id === expanded);
  const freshExamples = checkedVersion === state.current.id && checkedConfig === JSON.stringify(config) ? examples : undefined;
  async function check() {
    setChecking(true); setError("");
    try {
      const result = await filterRequest<{ naming_examples: NamingExample[] }>("/preview", { base_version: state.current.id, config });
      setExamples(result.naming_examples || []); setCheckedConfig(JSON.stringify(config)); setCheckedVersion(state.current.id);
    } catch (error) { setError(error instanceof Error ? error.message : "Could not check changes."); }
    finally { setChecking(false); }
  }
  return <section aria-labelledby={embedded ? undefined : "presentation-title"} className="space-y-3">
    {!embedded && <div className="flex items-center gap-2"><h2 id="presentation-title">Presentation</h2><FieldHelp label="Presentation">Choose how music names and groups appear in the view.</FieldHelp></div>}
    <div className="space-y-4 rounded-lg border border-(--color-bg-tertiary) bg-(--color-bg-secondary) p-4 sm:p-5">
      <div className="flex items-center gap-1"><h3 className="text-lg font-medium normal-case tracking-normal">{subtype === "direct" ? "Direct" : title}</h3><FieldHelp label={title}>{subtype === "direct" ? "Assign listening using information you know." : "Choose displayed names using original metadata and source information."}</FieldHelp></div>
      {rules.length > 0 && <div className="relative overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr className="text-(--color-fg-secondary)"><th className="p-2">Filter</th><th className="p-2">IF</th><th className="p-2">THEN</th><th className="p-2"><span className="sr-only">Enabled</span></th></tr></thead><tbody>
        {rules.map(rule => {
          const first = rule.program?.steps[0];
          const actions = (first?.if ? first.then : rule.program?.steps)?.map(statementSummary).join(", ") || "";
          return <tr key={rule.id} className="border-t border-(--color-bg-tertiary)">
            <td className="p-2"><button type="button" className="underline" disabled={busy} aria-expanded={expanded === rule.id} onClick={() => setExpanded(expanded === rule.id ? undefined : rule.id)}>{rule.name || `${title} filter`}</button></td>
            <td className="max-w-40 truncate p-2" title={expressionSummary(first?.if)}>{first?.if ? expressionSummary(first.if) : "TRUE"}</td>
            <td className="max-w-40 truncate p-2" title={actions}>{actions}</td>
            <td className="p-2"><Switch label={`Enable ${rule.name || `${title} filter`}`} checked={rule.enabled} disabled={busy} onChange={enabled => onChange({ ...rule, enabled })} /></td>
          </tr>;
        })}
      </tbody></table></div>}
      {current?.program && <div className="min-w-0 space-y-4 border-t border-(--color-bg-tertiary) pt-4" aria-label="Filter editor">
        <label className="block space-y-1 text-sm"><span>Name</span><input className={`${input} w-full`} value={current.name || ""} disabled={busy} maxLength={200} onChange={event => onChange({ ...current, name: event.target.value })} /></label>
        <details><summary className="cursor-pointer text-sm text-(--color-fg-secondary)">Priority</summary><label className="mt-2 flex items-center gap-2 text-sm">Priority<FieldHelp label="Filter priority">Higher priority wins when filters assign conflicting values.</FieldHelp><input type="number" min={0} max={100000} className={`${input} w-28`} disabled={busy} value={current.priority || 0} onChange={event => onChange({ ...current, priority: Number(event.target.value) })} /></label></details>
        <fieldset disabled={busy} className={`min-w-0 space-y-3 ${current.enabled ? "" : "opacity-50"}`}>
          <FilterBlockEditor program={current.program} schema={schema} operators={state.blocks.operators} disabled={busy} onChange={program => onChange({ ...current, program })} />
        </fieldset>
        <div className="flex flex-wrap gap-4 text-sm">
          {type === "presentation" && <button type="button" className={button} disabled={busy || checking} onClick={check}>{checking ? "Checking..." : "Check changes"}</button>}
          <button type="button" className="underline" disabled={busy} onClick={() => { const copy = { ...structuredClone(current), id: crypto.randomUUID(), name: `${current.name || "Naming filter"} copy` }; onChange(copy); setExpanded(copy.id); }}>Duplicate filter</button>
          <button type="button" className="underline" disabled={busy} onClick={() => { onRemove(current.id); setExpanded(undefined); }}>Remove filter</button>
        </div>
        {error && <p role="alert" className="text-sm text-(--color-error)">{error}</p>}
        {freshExamples && <div role="status" className="overflow-x-auto text-sm">
          {freshExamples.length === 0 ? <p>No displayed names change.</p> : <table className="w-full text-left"><thead><tr><th className="p-2">Field</th><th className="p-2">Before</th><th className="p-2">After</th></tr></thead><tbody>{freshExamples.map((example, i) => <tr key={i} className="border-t border-(--color-bg-tertiary)"><td className="p-2">{example.field}</td><td className="p-2">{example.before}</td><td className="p-2">{example.after}</td></tr>)}</tbody></table>}
        </div>}
      </div>}
      <button type="button" className={`${button} text-sm`} disabled={busy} onClick={() => {
        const direct = subtype === "direct";
        const rule: Rule = { id: crypto.randomUUID(), name: direct ? "Direct attribution" : "Naming filter", type, subtype, enabled: !direct, scope: {}, priority: 0, settings: { ...defaultRuleSettings }, program: { steps: [{ if: { op: "literal", value: true }, then: [{ set: direct ? "listener" : "display.title", value: direct ? { op: "literal", value: "unknown" } : { op: "field", field: "original.title" } }] }] } };
        onChange(rule); setExpanded(rule.id);
      }}>Add filter</button>
    </div>
  </section>;
}
