import { useEffect, useMemo, useState } from "react";
import FilterBlockEditor, { expressionSummary, statementSummary } from "./FilterBlockEditor";
import FieldHelp from "./FieldHelp";
import { Switch, filterInput as input, filterButton as button } from "./FilterControls";
import { defaultRuleSettings, filterRequest, type FilterState, type Rule, type RuleConfig } from "~/utils/filterRules";

type NamingExample = { record_id: number; field: string; before: string; after: string };

export default function ProgramFilterManager({ state, config, busy, type = "presentation", subtype = "naming", embedded = false, onChange, onRemove, onMove, focusId }: {
  state: FilterState; config: RuleConfig; busy: boolean; type?: string; subtype?: string; embedded?: boolean; onChange: (rule: Rule) => void; onRemove: (id: string) => void; onMove?: (id: string, direction: -1 | 1) => void; focusId?: string;
}) {
  const [expanded, setExpanded] = useState<string>();
  useEffect(() => { if (focusId) setExpanded(focusId); }, [focusId]);
  const [templateName, setTemplateName] = useState("");
  const [checking, setChecking] = useState(false);
  const [examples, setExamples] = useState<NamingExample[]>();
  const [checkedConfig, setCheckedConfig] = useState("");
  const [checkedVersion, setCheckedVersion] = useState(0);
  const [error, setError] = useState("");
  const baseSchema = state.blocks?.schemas[`${type}:${subtype}`];
  const schema = useMemo(() => {
    if (!baseSchema || !baseSchema.targets["time.zone"]) return baseSchema;
    const available = (Intl as typeof Intl & { supportedValuesOf?: (key: string) => string[] }).supportedValuesOf?.("timeZone") || ["America/Denver", "America/New_York", "America/Los_Angeles"];
    return { ...baseSchema, choices: { ...baseSchema.choices, "time.zone": [{ label: "Unknown", value: "" }, ...[...new Set(["UTC", ...available])].map(zone => ({ label: zone.replaceAll("_", " "), value: zone }))] } };
  }, [baseSchema]);
  if (!schema || !state.blocks) return null;
  const rules = config.rules.filter(rule => rule.type === type && rule.subtype === subtype);
  const templates = state.blocks.templates?.[`${type}:${subtype}`] || [];
  const template = templates.find(item => item.name === templateName) || templates[0];
  const title = type === "qualification" ? "Completion" : type === "inclusion" ? "Scope" : subtype === "direct" ? "Direct attribution" : subtype === "aggregate" ? "Aggregate attribution" : subtype === "heuristic" ? "Heuristic attribution" : type === "inference" && subtype === "count" ? "Count" : type === "inference" && subtype === "time" ? "Time" : type === "matching" ? subtype === "music" ? "Music matching" : "Play matching" : subtype === "grouping" ? "Grouping" : "Naming";
  const help = type === "qualification" ? "Choose which music history records count as plays using completion details and played duration." : type === "inclusion" ? "Choose music or individual plays to include. Later filters can include exceptions." : subtype === "direct" ? "Assign listening using information you know." : subtype === "aggregate" ? "Assign your percentage of a cumulative listening count. Whole plays round down." : subtype === "heuristic" ? "Estimate who listened using matching plays assigned to you by Direct filters." : type === "inference" && subtype === "count" ? "Choose how iTunes library export records contribute to totals." : type === "inference" && subtype === "time" ? "Determine when plays happened using each source's dates, durations, and timezone." : type === "matching" ? subtype === "music" ? "Choose comparison names and aliases for matching music and collecting track histories." : "Compare two records to determine whether they describe the same play. Left is the earlier record identifier." : subtype === "grouping" ? "Group album editions for reports while retaining each release and the album associated with each play." : "Choose displayed names using original metadata and source information.";
  const current = rules.find(rule => rule.id === expanded);
  const freshExamples = checkedVersion === state.current.id && checkedConfig === JSON.stringify(config) ? examples : undefined;
  async function check() {
    setChecking(true); setError("");
    try {
      const result = await filterRequest<{ scope_examples: NamingExample[]; naming_examples: NamingExample[]; music_examples: NamingExample[]; grouping_examples: NamingExample[] }>("/preview", { base_version: state.current.id, config });
      setExamples((type === "inclusion" ? result.scope_examples : type === "matching" ? result.music_examples : subtype === "grouping" ? result.grouping_examples : result.naming_examples) || []); setCheckedConfig(JSON.stringify(config)); setCheckedVersion(state.current.id);
    } catch (error) { setError(error instanceof Error ? error.message : "Could not check changes."); }
    finally { setChecking(false); }
  }
  return <section aria-labelledby={embedded ? undefined : "presentation-title"} className="space-y-3">
    {!embedded && <div className="flex items-center gap-2"><h2 id="presentation-title">Presentation</h2><FieldHelp label="Presentation">Choose how music names and groups appear in the view.</FieldHelp></div>}
    <div className="space-y-4 rounded-lg border border-(--color-bg-tertiary) bg-(--color-bg-secondary) p-4 sm:p-5">
      <div className="flex items-center gap-1"><h3 className="text-lg font-medium normal-case tracking-normal">{type === "attribution" ? title.replace(" attribution", "") : title}</h3><FieldHelp label={title}>{help}</FieldHelp></div>
      {rules.length > 0 && <div className="relative overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr className="text-(--color-fg-secondary)"><th className="p-2">Filter</th><th className="p-2">IF</th><th className="p-2">THEN</th><th className="p-2"><span className="sr-only">Enabled</span></th></tr></thead><tbody>
        {rules.map(rule => {
          const first = rule.program?.steps[0];
          const describe = (step: NonNullable<Rule["program"]>["steps"][number]) => statementSummary(step, schema, state.blocks?.operators);
          const condition = first?.if ? expressionSummary(first.if, schema, state.blocks?.operators) : "TRUE";
          const actions = first?.if ? `${(first.then || []).map(describe).join(", ")}${(first.else_if || []).map(branch => ` ELSE IF ${expressionSummary(branch.if, schema, state.blocks?.operators)} THEN ${branch.then.map(describe).join(", ")}`).join("")}${first.else ? ` ELSE ${first.else.map(describe).join(", ")}` : ""}${(rule.program?.steps.slice(1) || []).map(step => `, ${describe(step)}`).join("")}` : (rule.program?.steps || []).map(describe).join(", ");
          return <tr key={rule.id} className="border-t border-(--color-bg-tertiary)">
            <td className="p-2"><button type="button" className="underline" disabled={busy} aria-expanded={expanded === rule.id} onClick={() => setExpanded(expanded === rule.id ? undefined : rule.id)}>{rule.name || `${title} filter`}</button></td>
            <td className="max-w-40 truncate p-2" title={condition}>{condition}</td>
            <td className="max-w-40 truncate p-2" title={actions}>{actions}</td>
            <td className="p-2"><Switch label={`Enable ${rule.name || `${title} filter`}`} checked={rule.enabled} disabled={busy} onChange={enabled => onChange({ ...rule, enabled })} /></td>
          </tr>;
        })}
      </tbody></table></div>}
      {current?.program && <div className="min-w-0 space-y-4 border-t border-(--color-bg-tertiary) pt-4" aria-label="Filter editor">
        <label className="block space-y-1 text-sm"><span>Name</span><input className={`${input} w-full`} value={current.name || ""} disabled={busy} maxLength={200} onChange={event => onChange({ ...current, name: event.target.value })} /></label>
        {type !== "presentation" && type !== "qualification" && <details><summary className="cursor-pointer text-sm text-(--color-fg-secondary)">Priority</summary><label className="mt-2 flex items-center gap-2 text-sm">Priority<FieldHelp label="Filter priority">Higher priority wins when filters assign conflicting values.</FieldHelp><input type="number" min={0} max={100000} className={`${input} w-28`} disabled={busy} value={current.priority || 0} onChange={event => onChange({ ...current, priority: Number(event.target.value) })} /></label></details>}
        <fieldset disabled={busy} className={`min-w-0 space-y-3 ${current.enabled ? "" : "opacity-50"}`}>
          <FilterBlockEditor program={current.program} schema={schema} sources={state.sources} operators={state.blocks.operators} disabled={busy} onChange={program => onChange({ ...current, program })} />
        </fieldset>
        <div className="flex flex-wrap gap-4 text-sm">
          {(type === "presentation" || type === "inclusion" || type === "qualification") && onMove && <>
            <button type="button" className="underline disabled:opacity-50" disabled={busy || rules.indexOf(current) === 0} onClick={() => onMove(current.id, -1)}>Move up</button>
            <button type="button" className="underline disabled:opacity-50" disabled={busy || rules.indexOf(current) === rules.length - 1} onClick={() => onMove(current.id, 1)}>Move down</button>
          </>}
          {(type === "inclusion" || type === "presentation" || type === "matching" && subtype === "music") && <button type="button" className={button} disabled={busy || checking} onClick={check}>{checking ? "Checking..." : "Check changes"}</button>}
          <button type="button" className="underline" disabled={busy} onClick={() => { const copy = { ...structuredClone(current), id: crypto.randomUUID(), name: `${current.name || `${title} filter`} copy` }; onChange(copy); setExpanded(copy.id); }}>Duplicate filter</button>
          <button type="button" className="underline" disabled={busy} onClick={() => { onRemove(current.id); setExpanded(undefined); }}>Remove filter</button>
        </div>
        {error && <p role="alert" className="text-sm text-(--color-error)">{error}</p>}
        {freshExamples && <div role="status" className="overflow-x-auto text-sm">
          {freshExamples.length === 0 ? <p>{type === "inclusion" ? "No inclusion changes." : type === "matching" ? "No comparison values change." : subtype === "grouping" ? "No album groups change." : "No displayed names change."}</p> : <table className="w-full text-left"><thead><tr><th className="p-2">Field</th><th className="p-2">Before</th><th className="p-2">After</th></tr></thead><tbody>{freshExamples.map((example, i) => <tr key={i} className="border-t border-(--color-bg-tertiary)"><td className="p-2">{example.field}</td><td className="p-2">{example.before}</td><td className="p-2">{example.after}</td></tr>)}</tbody></table>}
        </div>}
      </div>}
      <div className="flex flex-wrap gap-2">
      {templates.length > 0 && <select aria-label={`${title} starting filter`} className={`${input} max-w-full text-sm`} disabled={busy} value={template?.name || ""} onChange={event => setTemplateName(event.target.value)}>{templates.map(item => <option key={item.name} value={item.name}>{item.label}</option>)}</select>}
      <button type="button" className={`${button} text-sm`} disabled={busy} onClick={() => {
        const direct = subtype === "direct", aggregate = subtype === "aggregate";
        const rule: Rule = { id: crypto.randomUUID(), name: `${title} filter`, type, subtype, enabled: type !== "attribution", scope: {}, priority: 0, settings: { ...defaultRuleSettings }, program: { steps: [{ if: aggregate ? { op: "eq", args: [{ op: "field", field: "source.service" }, { op: "literal", value: "itunes" }] } : { op: "literal", value: true }, then: [{ set: aggregate ? "share.percent" : direct ? "listener" : "display.title", value: aggregate ? { op: "number", args: [{ op: "literal", value: "0" }] } : direct ? { op: "literal", value: "unknown" } : { op: "field", field: "original.title" } }] }] } };
        if (type === "inclusion") {
          rule.enabled=false;
          rule.program={steps:[{if:{op:"literal",value:true},then:[{set:"view.include",value:{op:"literal",value:true}}]}]};
        }
        if (subtype === "grouping") {
          rule.enabled = false;
          rule.program = { steps: [{ if: { op: "literal", value: true }, then: [{ set: "group.album", value: { op: "field", field: "display.album" } }] }] };
        }
        if (subtype === "heuristic") {
          const collection = Object.keys(schema.fields).find(field => field.startsWith("evidence:"));
          rule.program = { steps: [{ if: collection ? { op: "ge", args: [{ op: "count", args: [{ op: "field", field: collection }] }, { op: "number", args: [{ op: "literal", value: "1" }] }] } : { op: "literal", value: false }, then: [{ set: "listener", value: { op: "literal", value: "self" } }], else: [{ set: "listener", value: { op: "literal", value: "unknown" } }] }] };
        }
        if (template) { rule.name = template.label; rule.enabled = false; rule.program = structuredClone(template.program); }
        onChange(rule); setExpanded(rule.id);
      }}>Add filter</button>
      </div>
    </div>
  </section>;
}
