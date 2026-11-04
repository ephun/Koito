import { filterButton as button, filterInput as input } from "./FilterControls";
import type { BlockOperator, BlockSchema, Expression, FilterProgram, FilterStatement } from "~/utils/filterRules";

const value = (v: unknown = ""): Expression => ({ op: "literal", value: v });
const number = (): Expression => ({ op: "number", args: [value("0")] });
const reference = (field: string): Expression => ({ op: "field", field });
const names: Record<string, string> = {
  "original.artist": "Original artist", "original.album": "Original album", "original.title": "Original track title",
  "display.artist": "Displayed artist", "display.album": "Displayed album", "display.title": "Displayed track title",
  "source.service": "Source", "source.id": "Source identifier", "file.hash": "File", "record.id": "Record identifier",
  "record.time": "Record time", "record.device": "Device", "library.id": "Library identifier",
  "track.persistent_id": "Library track identifier", "track.spotify_uri": "Spotify track URI", "track.recording_id": "MusicBrainz recording ID",
};
const label = (name: string) => names[name] || name;
const booleans = new Set(["and", "or", "not", "nor", "nand", "xor", "xnor"]);
const arithmetic = new Set(["add", "subtract", "multiply", "divide", "floor", "ceil", "absolute", "milliseconds", "seconds", "minutes", "hours", "days"]);

export function expressionSummary(e?: Expression): string {
  if (!e) return "";
  if (e.op === "field") return label(e.field || "");
  if (e.op === "literal") return typeof e.value === "boolean" ? String(e.value).toUpperCase() : JSON.stringify(e.value);
  return `${e.op.toUpperCase()}(${(e.args || []).map(expressionSummary).join(", ")})`;
}

export function statementSummary(step: FilterStatement): string {
  return step.if ? `IF ${expressionSummary(step.if)}` : `SET ${label(step.set || "")} TO ${expressionSummary(step.value)}`;
}

export default function FilterBlockEditor({ program, schema, operators, disabled, onChange }: {
  program: FilterProgram; schema: BlockSchema; operators: BlockOperator[]; disabled: boolean; onChange: (program: FilterProgram) => void;
}) {
  return <Statements steps={program.steps} schema={schema} operators={operators} disabled={disabled} onChange={steps => onChange({ steps })} />;
}

type Common = { schema: BlockSchema; operators: BlockOperator[]; disabled: boolean };

function ExpressionEditor({ expression, onChange, choices, ...common }: Common & { expression: Expression; onChange: (value: Expression) => void; choices?: { label: string; value: string }[] }) {
  const { schema, operators, disabled } = common;
  const args = expression.args || [];
  const operation = operators.find(op => op.id === expression.op);
  function choose(op: string) {
    if (op === "field") { onChange(reference(schema.fields["original.title"] ? "original.title" : Object.keys(schema.fields)[0])); return; }
    if (op === "literal") { onChange(value()); return; }
    const count = operators.find(item => item.id === op)?.min_args || 0;
    onChange({ op, args: Array.from({ length: count }, () => booleans.has(op) ? value(true) : arithmetic.has(op) ? number() : value()) });
  }
  return <div className="min-w-0 space-y-2 rounded border border-(--color-bg-tertiary) p-2">
    <div className="flex min-w-0 flex-wrap items-center gap-2">
      <select aria-label="Expression block" className={`${input} max-w-full text-sm`} disabled={disabled} value={expression.op} onChange={event => choose(event.target.value)}>
        <option value="field">Field</option><option value="literal">Value</option>
        {operators.map(op => <option key={op.id} value={op.id}>{op.label}</option>)}
      </select>
      {expression.op === "field" && <select aria-label="Field" className={`${input} flex-1 text-sm`} disabled={disabled} value={expression.field} onChange={event => onChange(reference(event.target.value))}>
        {Object.keys({ ...schema.fields, ...schema.targets }).map(field => <option key={field} value={field}>{label(field)}</option>)}
      </select>}
      {expression.op === "literal" && choices?.length ? <select aria-label="Value" className={`${input} min-w-0 flex-1 text-sm`} disabled={disabled} value={String(expression.value ?? "")} onChange={event => onChange(value(event.target.value))}>
        {!choices.some(choice => choice.value === expression.value) && <option value={String(expression.value ?? "")}>{expression.value ? String(expression.value) : "Choose value"}</option>}
        {choices.map(choice => <option key={choice.value} value={choice.value}>{choice.label}</option>)}
      </select> : expression.op === "literal" && <>
        <select aria-label="Value type" className={`${input} text-sm`} disabled={disabled} value={expression.value === null ? "null" : typeof expression.value} onChange={event => onChange(event.target.value === "number" ? number() : value(event.target.value === "boolean" ? true : event.target.value === "null" ? null : ""))}>
          <option value="string">Text</option><option value="number">Number</option><option value="boolean">Boolean</option><option value="null">Null</option>
        </select>
        {typeof expression.value === "boolean" ? <select aria-label="Boolean value" className={`${input} text-sm`} disabled={disabled} value={String(expression.value)} onChange={event => onChange(value(event.target.value === "true"))}><option value="true">TRUE</option><option value="false">FALSE</option></select>
          : expression.value !== null && <input aria-label="Value" className={`${input} w-full flex-1 text-sm`} disabled={disabled} type={typeof expression.value === "number" ? "number" : "text"} step="any" value={String(expression.value ?? "")} onChange={event => onChange(value(typeof expression.value === "number" ? Number(event.target.value) : event.target.value))} />}
      </>}
    </div>
    {args.length > 0 && <div className="space-y-2 border-l-2 border-(--color-bg-tertiary) pl-2 sm:pl-3">
      {args.map((arg, index) => <div key={index} className="min-w-0 space-y-1">
        <ExpressionEditor {...common} choices={index === 1 && ["eq", "ne"].includes(expression.op) && args[0]?.op === "field" ? schema.choices?.[args[0].field || ""] : undefined} expression={arg} onChange={next => onChange({ ...expression, args: args.map((existing, i) => i === index ? next : existing) })} />
        {!!operation && args.length > operation.min_args && <button type="button" disabled={disabled} className="text-xs underline" onClick={() => onChange({ ...expression, args: args.filter((_, i) => i !== index) })}>Remove input</button>}
      </div>)}
    </div>}
    {!!operation && args.length < operation.max_args && <button type="button" disabled={disabled} className="text-sm underline" onClick={() => onChange({ ...expression, args: [...args, value(booleans.has(expression.op) ? true : "")] })}>Add input</button>}
  </div>;
}

function Statements({ steps, onChange, ...common }: Common & { steps: FilterStatement[]; onChange: (steps: FilterStatement[]) => void }) {
  const { schema, disabled } = common;
  function edit(index: number, statement: FilterStatement) { onChange(steps.map((existing, i) => i === index ? statement : existing)); }
  function move(index: number, by: number) { const next = [...steps]; [next[index], next[index + by]] = [next[index + by], next[index]]; onChange(next); }
  return <div className="min-w-0 space-y-3">
    {steps.map((step, index) => <div key={index} className="min-w-0 space-y-3 rounded border border-(--color-bg-tertiary) p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-sm font-semibold">{step.if ? "IF" : "SET"}</span>
        <div className="flex gap-3 text-xs">
          {index > 0 && <button type="button" disabled={disabled} onClick={() => move(index, -1)}>Move up</button>}
          {index < steps.length - 1 && <button type="button" disabled={disabled} onClick={() => move(index, 1)}>Move down</button>}
          <button type="button" disabled={disabled} onClick={() => onChange(steps.filter((_, i) => i !== index))}>Remove block</button>
        </div>
      </div>
      {step.if ? <>
        <ExpressionEditor {...common} expression={step.if} onChange={next => edit(index, { ...step, if: next })} />
        <p className="text-sm font-semibold">THEN</p>
        <Statements {...common} steps={step.then || []} onChange={then => edit(index, { ...step, then })} />
        {(step.else_if || []).map((branch, i) => <div key={i} className="space-y-2">
          <div className="flex justify-between gap-2"><span className="text-sm font-semibold">ELSE IF</span><button type="button" className="text-xs" disabled={disabled} onClick={() => edit(index, { ...step, else_if: step.else_if?.filter((_, j) => j !== i) })}>Remove branch</button></div>
          <ExpressionEditor {...common} expression={branch.if} onChange={next => edit(index, { ...step, else_if: step.else_if?.map((existing, j) => j === i ? { ...existing, if: next } : existing) })} />
          <p className="text-sm font-semibold">THEN</p>
          <Statements {...common} steps={branch.then} onChange={then => edit(index, { ...step, else_if: step.else_if?.map((existing, j) => j === i ? { ...existing, then } : existing) })} />
        </div>)}
        {step.else !== undefined && <>
          <div className="flex justify-between gap-2"><span className="text-sm font-semibold">ELSE</span><button type="button" className="text-xs" disabled={disabled} onClick={() => { const next = { ...step }; delete next.else; edit(index, next); }}>Remove ELSE</button></div>
          <Statements {...common} steps={step.else} onChange={otherwise => edit(index, { ...step, else: otherwise })} />
        </>}
        <div className="flex flex-wrap gap-3 text-sm">
          <button type="button" disabled={disabled} onClick={() => edit(index, { ...step, else_if: [...(step.else_if || []), { if: value(true), then: [] }] })}>Add ELSE IF</button>
          {step.else === undefined && <button type="button" disabled={disabled} onClick={() => edit(index, { ...step, else: [] })}>Add ELSE</button>}
        </div>
      </> : <>
        <select aria-label="SET field" className={`${input} w-full text-sm`} value={step.set} disabled={disabled} onChange={event => edit(index, { ...step, set: event.target.value })}>
          {Object.keys(schema.targets).map(field => <option key={field} value={field}>{label(field)}</option>)}
        </select>
        <p className="text-sm font-semibold">TO</p>
        <ExpressionEditor {...common} expression={step.value || value()} onChange={next => edit(index, { ...step, value: next })} />
      </>}
    </div>)}
    <div className="flex flex-wrap gap-2">
      <button type="button" className={`${button} text-sm`} disabled={disabled} onClick={() => onChange([...steps, { if: value(true), then: [] }])}>Add IF</button>
      <button type="button" className={`${button} text-sm`} disabled={disabled} onClick={() => { const target = schema.targets["display.title"] ? "display.title" : Object.keys(schema.targets)[0]; onChange([...steps, { set: target, value: reference(target) }]); }}>Add SET</button>
    </div>
  </div>;
}
