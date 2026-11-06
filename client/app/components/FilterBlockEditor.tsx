import { useEffect, useMemo, useState } from "react";
import FieldHelp from "./FieldHelp";
import ArtistCreditList from "./ArtistCreditList";
import { filterButton as button, filterInput as input } from "./FilterControls";
import { sourceName, type BlockOperator, type BlockSchema, type Expression, type FilterProgram, type FilterStatement, type FilterSource, type SourceField } from "~/utils/filterRules";

const value = (v: unknown = ""): Expression => ({ op: "literal", value: v });
const number = (): Expression => ({ op: "number", args: [value("0")] });
const reference = (field: string): Expression => ({ op: "field", field });
const names: Record<string, string> = {
	"item": "Item",
	"share.percent": "My share (%)",
  "listener": "Listener", "range.from": "Evidence interval start", "range.until": "Evidence interval end",
  "original.artist": "Original artist", "original.album": "Original album", "original.title": "Original track title",
  "display.artist": "Displayed artist", "display.album": "Displayed album", "display.title": "Displayed track title",
  "source.service": "Source", "source.id": "Source identifier", "file.hash": "File", "record.id": "Record identifier",
  "record.time": "Record time", "record.device": "Device", "library.id": "Library identifier",
  "track.persistent_id": "Library track identifier", "track.spotify_uri": "Spotify track URI", "track.recording_id": "MusicBrainz recording ID",
};
const label = (name: string): string => names[name] || (/^(left|right)\./.test(name) ? `${name.startsWith("left.") ? "Left" : "Right"} ${label(name.slice(name.indexOf(".") + 1))}` : name.startsWith("evidence:") ? `Personal plays (source ${name.slice(9)})` : /^(raw:|column:)/.test(name) ? name.slice(name.indexOf(":") + 1).split("/").filter(Boolean).map(part => part.replaceAll("~1", "/").replaceAll("~0", "~")).join(" / ") || "Source record" : name);
const booleans = new Set(["and", "or", "not", "nor", "nand", "xor", "xnor"]);
const arithmetic = new Set(["add", "subtract", "multiply", "divide", "floor", "ceil", "absolute", "milliseconds", "seconds", "minutes", "hours", "days"]);
const operatorHelp: Record<string, string> = {
  where: "Keep items whose condition is TRUE. Use Item to read the current list entry.",
  map: "Calculate one value for each item. Use Item to read the current list entry.",
  get: "Read an object field or list position. List positions start at zero.",
};

function fieldChoices(schema: BlockSchema | undefined, expression: Expression) {
  const name = expression.field || "";
  const key = ({ "item:/file_hash": "file.hash", "item:/source_id": "source.id" } as Record<string, string>)[name] || name;
  return schema?.choices?.[key];
}

export function expressionSummary(e?: Expression, schema?: BlockSchema, operators: BlockOperator[] = [], choices?: { label: string; value: string }[]): string {
  if (!e) return "";
  if (e.op === "field") return schema?.labels?.[e.field || ""] || schema?.item_labels?.[e.field || ""] || label(e.field || "");
  if (e.op === "literal") return choices?.find(choice => choice.value === e.value)?.label || (typeof e.value === "boolean" ? String(e.value).toUpperCase() : JSON.stringify(e.value));
  if (e.op === "number" && e.args?.[0]?.op === "literal" && typeof e.args[0].value === "string" && /^-?\d+(\.\d+)?$/.test(e.args[0].value)) return e.args[0].value;
  const args = (e.args || []).map((arg, index) => expressionSummary(arg, schema, operators, index === 1 && e.args?.[0]?.op === "field" ? fieldChoices(schema, e.args[0]) : undefined));
  const name = operators.find(operator => operator.id === e.op)?.label || e.op.toUpperCase();
  return ["eq", "ne", "lt", "le", "gt", "ge", "add", "subtract", "multiply", "divide"].includes(e.op) && args.length === 2 ? `(${args[0]} ${name} ${args[1]})` : `${name}(${args.join(", ")})`;
}

export function statementSummary(step: FilterStatement, schema?: BlockSchema, operators: BlockOperator[] = []): string {
  const statements = (steps: FilterStatement[] = []) => steps.map(child => statementSummary(child, schema, operators)).join(", ");
  return step.if ? `IF ${expressionSummary(step.if, schema, operators)} THEN ${statements(step.then)}${(step.else_if || []).map(branch => ` ELSE IF ${expressionSummary(branch.if, schema, operators)} THEN ${statements(branch.then)}`).join("")}${step.else ? ` ELSE ${statements(step.else)}` : ""}` : `SET ${schema?.labels?.[step.set || ""] || label(step.set || "")} TO ${expressionSummary(step.value, schema, operators, schema?.choices?.[step.set || ""])}`;
}

export default function FilterBlockEditor({ program, schema, operators, disabled, sources = [], onChange }: {
  program: FilterProgram; schema: BlockSchema; operators: BlockOperator[]; disabled: boolean; sources?: FilterSource[]; onChange: (program: FilterProgram) => void;
}) {
  const [source, setSource] = useState("");
  const [fields, setFields] = useState<SourceField[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (!source) return;
    const controller = new AbortController();
    setLoading(true); setError("");
    async function load() {
      try {
        const response = await fetch(`/apis/web/v1/filters/fields?source=${encodeURIComponent(source)}`, { signal: controller.signal });
        const data = await response.json();
        if (!response.ok) throw new Error(data.error || "Could not load source fields.");
        setFields(existing => [...existing, ...data]);
      } catch (error) {
        if (!controller.signal.aborted) setError(error instanceof Error ? error.message : "Could not load source fields.");
      } finally { if (!controller.signal.aborted) setLoading(false); }
    }
    load();
    return () => controller.abort();
  }, [source]);
  const editingSchema = useMemo(() => {
    const next = { ...schema, fields: { ...schema.fields }, labels: { ...schema.labels }, item_fields: { ...schema.item_fields }, item_labels: { ...schema.item_labels } };
    for (const field of fields) {
      if (schema.snapshot_fields && field.path.startsWith("raw:")) { const key = `item:/raw${field.path.slice(4)}`; next.item_fields[key] = "any"; next.item_labels[key] = field.label; }
      else if (schema.pair_fields) { for (const side of ["left", "right"]) { next.fields[`${side}.${field.path}`] = "any"; next.labels[`${side}.${field.path}`] = `${side === "left" ? "Left" : "Right"} ${field.label}`; } }
      else if (!schema.snapshot_fields) { next.fields[field.path] = "any"; next.labels[field.path] = field.label; }
    }
    // Keep saved references selectable before their source catalog is loaded.
    function visit(value: unknown) {
      if (!value || typeof value !== "object") return;
      const item = value as Record<string, unknown>;
      if (item.op === "field" && typeof item.field === "string") { if (/^(raw:|column:|evidence:|(left|right)\.(raw:|column:))/.test(item.field)) next.fields[item.field] = item.field.startsWith("evidence:") ? "list" : "any"; if (item.field.startsWith("item:")) next.item_fields[item.field] ||= "any"; }
      for (const child of Object.values(item)) visit(child);
    }
    visit(program);
    return next;
  }, [schema, fields, program]);
  const selectableSources = sources.filter(item => item.records > 0 && (!schema.snapshot_fields || ["itunes", "navidrome"].includes(item.service)));
  return <div className="min-w-0 space-y-3">
    {(schema.raw_fields || schema.snapshot_fields || schema.pair_fields) && sources.length > 0 && <details>
      <summary className="cursor-pointer text-sm">{schema.snapshot_fields ? "Record fields" : "Source fields"}</summary>
      {schema.snapshot_fields && selectableSources.length === 1 ? <button type="button" className="mt-2 text-sm underline" disabled={disabled || loading || !!fields.length} onClick={() => setSource(String(selectableSources[0].id))}>Load fields</button> :
      <label className="mt-2 flex min-w-0 flex-wrap items-center gap-2 text-sm">Source<FieldHelp label="Source fields">Choose a source to add its imported metadata to the field menu.</FieldHelp>
        <select aria-label="Source fields" className={`${input} max-w-full`} value={source} disabled={disabled || loading} onChange={event => setSource(event.target.value)}>
          <option value="">Choose source</option>{selectableSources.map(item => <option key={item.id} value={item.id}>{sourceName(item.service,item.label)}</option>)}
        </select>
      </label>}
      {loading && <p role="status" className="mt-2 text-sm">Loading fields...</p>}
      {error && <p role="alert" className="mt-2 text-sm">{error}</p>}
    </details>}
    <Statements steps={program.steps} schema={editingSchema} operators={operators} disabled={disabled} onChange={steps => onChange({ steps })} />
  </div>;
}

type Common = { schema: BlockSchema; operators: BlockOperator[]; disabled: boolean };

export function ExpressionEditor({ expression, onChange, choices, artistCredits, ...common }: Common & { expression: Expression; onChange: (value: Expression) => void; choices?: { label: string; value: string }[]; artistCredits?: boolean }) {
  const { schema, operators, disabled } = common;
  const args = expression.args || [];
  const operation = operators.find(op => op.id === expression.op);
  const creditIDs = !artistCredits ? undefined : expression.op === "list" && args.every(arg => arg.op === "literal" && typeof arg.value === "number") ? args.map(arg => arg.value as number) : expression.op === "literal" && Array.isArray(expression.value) && expression.value.every(item => typeof item === "number") ? expression.value as number[] : undefined;
  const creditList = creditIDs !== undefined;
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
      {operatorHelp[expression.op] && <FieldHelp label={operation?.label || expression.op}>{operatorHelp[expression.op]}</FieldHelp>}
      {expression.op === "field" && <select aria-label="Field" className={`${input} flex-1 text-sm`} disabled={disabled} value={expression.field} onChange={event => onChange(reference(event.target.value))}>
        {Object.keys({ ...schema.fields, ...schema.targets }).map(field => <option key={field} value={field}>{schema.labels?.[field] || label(field)}</option>)}
      </select>}
      {expression.op === "field" && schema.help?.[expression.field || ""] && <FieldHelp label={schema.labels?.[expression.field || ""] || label(expression.field || "")}>{schema.help[expression.field || ""]}</FieldHelp>}
      {expression.op === "field" && expression.field?.startsWith("evidence:") && <FieldHelp label="Personal plays">Collections contain distinct matching plays assigned to you by Direct filters. Item fields include time, artist, album, title, and device.</FieldHelp>}
      {!creditList && expression.op === "literal" && choices?.length ? <select aria-label="Value" className={`${input} min-w-0 flex-1 text-sm`} disabled={disabled} value={String(expression.value ?? "")} onChange={event => onChange(value(event.target.value))}>
        {!choices.some(choice => choice.value === expression.value) && <option value={String(expression.value ?? "")}>{expression.value ? String(expression.value) : "Choose value"}</option>}
        {choices.map(choice => <option key={choice.value} value={choice.value}>{choice.label}</option>)}
      </select> : !creditList && expression.op === "literal" && <>
        <select aria-label="Value type" className={`${input} text-sm`} disabled={disabled} value={expression.value === null ? "null" : typeof expression.value} onChange={event => onChange(event.target.value === "number" ? number() : value(event.target.value === "boolean" ? true : event.target.value === "null" ? null : ""))}>
          <option value="string">Text</option><option value="number">Number</option><option value="boolean">Boolean</option><option value="null">Null</option>
        </select>
        {typeof expression.value === "boolean" ? <select aria-label="Boolean value" className={`${input} text-sm`} disabled={disabled} value={String(expression.value)} onChange={event => onChange(value(event.target.value === "true"))}><option value="true">TRUE</option><option value="false">FALSE</option></select>
          : expression.value !== null && <input aria-label="Value" className={`${input} w-full flex-1 text-sm`} disabled={disabled} type={typeof expression.value === "number" ? "number" : "text"} step="any" value={String(expression.value ?? "")} onChange={event => onChange(value(typeof expression.value === "number" ? Number(event.target.value) : event.target.value))} />}
      </>}
    </div>
    {creditList && <ArtistCreditList ids={creditIDs || []} disabled={disabled} onChange={ids => onChange({op:"list",args:ids.map(id => value(id))})} />}
    {!creditList && args.length > 0 && <div className="space-y-2 border-l-2 border-(--color-bg-tertiary) pl-2 sm:pl-3">
      {args.map((arg, index) => <div key={index} className="min-w-0 space-y-1">
        <ExpressionEditor {...common} schema={index === 1 && ["where", "map"].includes(expression.op) ? { ...schema, fields: { ...schema.fields, item: "any", ...(args[0]?.op === "field" && ["library.snapshots", "count.snapshots"].includes(args[0].field || "") ? schema.item_fields : {}) }, labels: { ...schema.labels, ...schema.item_labels } } : schema} choices={index === 1 && ["eq", "ne"].includes(expression.op) && args[0]?.op === "field" ? fieldChoices(schema, args[0]) : undefined} expression={arg} onChange={next => onChange({ ...expression, args: args.map((existing, i) => i === index ? next : existing) })} />
        {!!operation && args.length > operation.min_args && <button type="button" disabled={disabled} className="text-xs underline" onClick={() => onChange({ ...expression, args: args.filter((_, i) => i !== index) })}>Remove input</button>}
      </div>)}
    </div>}
    {!creditList && !!operation && args.length < operation.max_args && <button type="button" disabled={disabled} className="text-sm underline" onClick={() => onChange({ ...expression, args: [...args, value(booleans.has(expression.op) ? true : "")] })}>Add input</button>}
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
        <div className="flex min-w-0 items-center gap-2"><select aria-label="SET field" className={`${input} w-full text-sm`} value={step.set} disabled={disabled} onChange={event => { const target = event.target.value; edit(index, { ...step, set: target, ...(schema.targets[target] === "list" && schema.targets[step.set || ""] !== "list" ? { value: { op: "list", args: [] } } : {}) }); }}>
          {Object.keys(schema.targets).map(field => <option key={field} value={field}>{schema.labels?.[field] || label(field)}</option>)}
        </select>{schema.help?.[step.set || ""] && <FieldHelp label={schema.labels?.[step.set || ""] || label(step.set || "")}>{schema.help[step.set || ""]}</FieldHelp>}</div>
        <p className="text-sm font-semibold">TO</p>
        <ExpressionEditor {...common} artistCredits={["display.artist_ids","display.primary_artist_ids"].includes(step.set || "")} choices={schema.choices?.[step.set || ""]} expression={step.value || value()} onChange={next => edit(index, { ...step, value: next })} />
      </>}
    </div>)}
    <div className="flex flex-wrap gap-2">
      <button type="button" className={`${button} text-sm`} disabled={disabled} onClick={() => onChange([...steps, { if: value(true), then: [] }])}>Add IF</button>
      <button type="button" className={`${button} text-sm`} disabled={disabled} onClick={() => { const target = schema.targets["display.title"] ? "display.title" : Object.keys(schema.targets)[0]; onChange([...steps, { set: target, value: schema.targets[target] === "number" ? number() : schema.targets[target] === "boolean" ? value(false) : target === "listener" ? value("unknown") : reference(target) }]); }}>Add SET</button>
    </div>
  </div>;
}
