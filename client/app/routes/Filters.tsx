import { useEffect, useState } from "react";
import { Navigate, useSearchParams } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import FieldHelp from "~/components/FieldHelp";
import ProgramFilterManager from "~/components/ProgramFilterManager";
import { useAppContext } from "~/providers/AppProvider";
import { filterButton as button } from "~/components/FilterControls";
import { defaultRuleSettings, filterRequest, FilterRequestError, type FilterState, type Rule, type RuleConfig, type RuleVersion } from "~/utils/filterRules";

export default function Filters() {
  const { user } = useAppContext();
  const queries = useQueryClient();
  const [params] = useSearchParams();
  const recordID = params.get("record");
  const listenTrackID=Number(params.get("listen_track")) || 0;
  const listenTime=params.get("listen_time") || "";
  const listenKey=params.get("listen_key") || "";
  const playRecordID=Number(params.get("play_record")) || 0;
  const artistID = Number(params.get("artist")) || 0;
  const catalogKind = params.get("catalog") || "";
  const catalogID = Number(params.get("id")) || 0;
  const catalogEdit = params.get("edit") || "name";
  const [saved, setSaved] = useState<FilterState>();
  const [draft, setDraft] = useState<RuleConfig>();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [stale, setStale] = useState(false);
  const [message, setMessage] = useState("");
  const [focusedFilter, setFocusedFilter] = useState<string>();
  const [focusedMatching, setFocusedMatching] = useState(false);
  const [focusedGrouping, setFocusedGrouping] = useState(false);
  const [focusedScope, setFocusedScope] = useState(false);

  async function load() {
    setLoading(true);
    setError("");
    setStale(false);
    setFocusedMatching(false);
    setFocusedGrouping(false);
    setFocusedScope(false);
    setFocusedFilter(undefined);
    try {
      const state = await filterRequest<FilterState>();
      state.current.config.rules = state.current.config.rules.map(rule => state.blocks?.legacy?.[rule.id] || rule);
      state.current.config.rules.push(...(state.blocks?.additions || []).filter(rule => !state.current.config.rules.some(existing => existing.id === rule.id)));
      state.sources ||= [];
      state.files ||= [];
      state.libraries ||= [];
      setSaved(state);
      const next = structuredClone(state.current.config);
      if (artistID > 0) {
        const existing = next.rules.find(rule => rule.type === "presentation" && rule.subtype === "naming" && rule.program?.steps.some(step => step.if?.op === "eq" && step.if.args?.[0]?.field === "catalog.artist_id" && step.if.args?.[1]?.value === artistID && step.then?.some(action => action.set === "artist.genres")));
        if (existing) setFocusedFilter(existing.id);
        else {
          const created: Rule = {id: crypto.randomUUID(), name: "Artist genres", type: "presentation", subtype: "naming", enabled: true, scope: {}, settings: {...defaultRuleSettings}, program: {steps: [{if: {op: "eq", args: [{op:"field",field:"catalog.artist_id"},{op:"literal",value:artistID}]}, then: [{set:"artist.genres",value:{op:"field",field:"source.genres"}}]}]}};
          next.rules.push(created);
          setFocusedFilter(created.id);
        }
      }

      if (["artist", "album", "track"].includes(catalogKind) && catalogID > 0) {
        const identifier = catalogEdit === "identifier";
        const classification = catalogKind === "album" && catalogEdit === "classification";
        const credits = ["album","track"].includes(catalogKind) && catalogEdit === "credits";
        const aliases = catalogEdit === "aliases";
        const scope = catalogEdit === "scope";
        const id = `catalog-${scope ? "scope" : aliases ? "aliases" : credits ? "credits" : classification ? "classification" : identifier ? "identifier" : "name"}-${catalogKind}-${catalogID}`;
        setFocusedMatching(identifier);
        setFocusedGrouping(classification);
        setFocusedScope(scope);
        const existing = next.rules.find(rule => rule.id === id);
        if (!existing) {
          const response = await fetch(`/apis/web/v1/${catalogKind}/${catalogID}`);
          if (!response.ok) throw new Error("Could not load music.");
          const item = await response.json();
          const field = catalogKind === "track" ? "title" : catalogKind;
          const name = catalogKind === "artist" ? item.name : item.title;
          if (scope) {
            const condition = catalogKind === "artist" || catalogKind === "track" ? {op:"in",args:[{op:"literal",value:catalogID},{op:"field",field:catalogKind === "artist" ? "catalog.artist_ids" : "catalog.track_ids"}]} : {op:"eq",args:[{op:"field",field:"catalog.album_id"},{op:"literal",value:catalogID}]};
            next.rules.push({id,name:`Inclusion for ${name}`.slice(0,200),type:"inclusion",subtype:"scope",enabled:true,scope:{},settings:{...defaultRuleSettings},program:{steps:[{if:condition,then:[{set:"view.include",value:{op:"literal",value:false}}]}]}});
          } else
          if (aliases) {
            const response = await fetch(`/apis/web/v1/${catalogKind}/${catalogID}/aliases`);
            if (!response.ok) throw new Error("Could not load aliases.");
            const names = await response.json() as {alias:string}[];
            next.rules.push({id,name:`Aliases for ${name}`.slice(0,200),type:"presentation",subtype:"naming",enabled:true,scope:{},settings:{...defaultRuleSettings},program:{steps:[{if:{op:"and",args:[{op:"eq",args:[{op:"field",field:"catalog.kind"},{op:"literal",value:catalogKind}]},{op:"eq",args:[{op:"field",field:"catalog.id"},{op:"literal",value:catalogID}]}]},then:[{set:"display.aliases",value:{op:"list",args:(names || []).map(item=>({op:"literal",value:item.alias}))}}]}]}});
          } else
          if (credits) {
            const response = await fetch(`/apis/web/v1/${catalogKind}/${catalogID}/artists`);
            if (!response.ok) throw new Error("Could not load artist credits.");
            const artists = await response.json() as {id:number;is_primary:boolean}[];
            next.rules.push({id,name:`Artist credits for ${name}`.slice(0,200),type:"presentation",subtype:"naming",enabled:true,scope:{},settings:{...defaultRuleSettings},program:{steps:[{if:{op:"and",args:[{op:"eq",args:[{op:"field",field:"catalog.kind"},{op:"literal",value:catalogKind}]},{op:"eq",args:[{op:"field",field:"catalog.id"},{op:"literal",value:catalogID}]}]},then:[{set:"display.artist_ids",value:{op:"list",args:(artists || []).map(artist=>({op:"literal",value:artist.id}))}},{set:"display.primary_artist_ids",value:{op:"list",args:(artists || []).filter(artist=>artist.is_primary).map(artist=>({op:"literal",value:artist.id}))}}]}]}});
          } else
          next.rules.push({id, name: `${classification ? "Various Artists" : identifier ? "MusicBrainz identifier" : "Name"} for ${name}`.slice(0,200), type:identifier ? "matching" : "presentation", subtype:classification ? "grouping" : identifier ? "music" : "naming", enabled:true, scope:{}, settings:{...defaultRuleSettings}, program:{steps:[{if:{op:"and",args:[{op:"eq",args:[{op:"field",field:"catalog.kind"},{op:"literal",value:catalogKind}]},{op:"eq",args:[{op:"field",field:"catalog.id"},{op:"literal",value:catalogID}]}]},then:[{set:classification ? "group.various_artists" : identifier ? "music.identifier" : `display.${field}`,value:{op:"literal",value:classification ? item.is_various_artists : identifier ? item.musicbrainz_id || "" : name}}]}]}});
        }
        setFocusedFilter(id);
      }

      if (listenTrackID>0 && Number.isFinite(Date.parse(listenTime))) {
        const identity=listenKey || JSON.stringify([listenTrackID,new Date(listenTime).toISOString()]);
        const digest=await crypto.subtle.digest("SHA-256",new TextEncoder().encode(identity));
        const id=playRecordID>0 ? `play-record-${playRecordID}` : `play-${Array.from(new Uint8Array(digest),byte=>byte.toString(16).padStart(2,"0")).join("")}`;
        if (!next.rules.some(rule=>rule.id===id)) {
          const response=await fetch(`/apis/web/v1/track/${listenTrackID}`);
          if (!response.ok) throw new Error("Could not load music.");
          const track=await response.json();
          const condition=playRecordID>0 ? {op:"in",args:[{op:"literal",value:playRecordID},{op:"field",field:"play.record_ids"}]} : listenKey ? {op:"eq",args:[{op:"field",field:"play.key"},{op:"literal",value:listenKey}]} : {op:"and",args:[{op:"eq",args:[{op:"field",field:"catalog.appearance_id"},{op:"literal",value:listenTrackID}]},{op:"eq",args:[{op:"field",field:"play.time"},{op:"date",args:[{op:"literal",value:listenTime}]}]},{op:"eq",args:[{op:"field",field:"play.key"},{op:"literal",value:listenKey}]}]};
          next.rules.push({id,name:`Play inclusion for ${track.title}`.slice(0,200),type:"inclusion",subtype:"scope",enabled:true,scope:{},settings:{...defaultRuleSettings},program:{steps:[{if:condition,then:[{set:"view.include",value:{op:"literal",value:false}}]}]}});
        }
        setFocusedScope(true);setFocusedFilter(id);
      }
      setDraft(next);
    } catch (error) {
      setError(errorMessage(error));
    } finally { setLoading(false); }
  }

  useEffect(() => {
    if (user?.role === "admin" && !recordID) void load();
  }, [user?.role, recordID, artistID, catalogKind, catalogID, catalogEdit, listenTrackID, listenTime, listenKey, playRecordID]);

  function upsert(rule: Rule) {
    setDraft(previous => previous && ({ rules: previous.rules.some(existing => existing.id === rule.id)
      ? previous.rules.map(existing => existing.id === rule.id ? rule : existing)
      : [...previous.rules, rule] }));
    setMessage("");
  }
  function remove(id: string) {
    setDraft(previous => previous && ({ rules: previous.rules.filter(rule => rule.id !== id) }));
    setMessage("");
  }

  function move(id: string, direction: -1 | 1) {
    setDraft(previous => {
      if (!previous) return previous;
      const rules = [...previous.rules];
      const index = rules.findIndex(rule => rule.id === id);
      const rule = rules[index];
      if (!rule) return previous;
      let other = index + direction;
      while (other >= 0 && other < rules.length && (rules[other].type !== rule.type || rules[other].subtype !== rule.subtype)) other += direction;
      if (other < 0 || other >= rules.length) return previous;
      [rules[index], rules[other]] = [rules[other], rules[index]];
      return { rules };
    });
    setMessage("");
  }

  async function save() {
    if (!saved || !draft || saving) return;
    setSaving(true);
    setError("");
    setMessage("");
    setStale(false);
    try {
      const version = await filterRequest<RuleVersion>("/save", { base_version: saved.current.id, config: draft });
      setSaved({ ...saved, current: version });
      setDraft(structuredClone(version.config));
      await queries.invalidateQueries();
      setMessage("Saved.");
    } catch (error) {
      setError(errorMessage(error));
      setStale(error instanceof FilterRequestError && error.status === 409);
    } finally { setSaving(false); }
  }

  const dirty = !!saved && !!draft && JSON.stringify(saved.current.config) !== JSON.stringify(draft);

  if (recordID) return <Navigate replace to={`/imports?record=${encodeURIComponent(recordID)}`} />;
  if (user?.role !== "admin") return <main className="p-6"><h1>Filters</h1><p>Administrator login required.</p></main>;

  return <main className="w-full max-w-4xl space-y-8 p-4 sm:p-8">
    <title>Filters | Koito</title>
    <header className="flex flex-wrap items-center justify-between gap-4">
      <h1>Filters</h1>
      <div className="flex items-center gap-3">
        {dirty && <button type="button" className="px-2 py-2 text-(--color-fg-secondary) hover:underline disabled:opacity-50" disabled={saving} onClick={() => { setDraft(structuredClone(saved!.current.config)); setMessage(""); setError(""); }}>Discard</button>}
        <button type="button" className={button} disabled={!dirty || saving || loading} onClick={save}>{saving ? "Saving..." : "Save changes"}</button>
      </div>
    </header>

    {error && <div role="alert" className="rounded border border-(--color-error) p-4">
      <p>{error}</p>
      {(stale || !saved) && <button type="button" className="mt-2 underline" onClick={load}>Reload</button>}
    </div>}
    {loading && <p role="status">Loading filters...</p>}
    {message && <p role="status" className="text-sm text-(--color-fg-secondary)">{message}</p>}

    {saved && draft && <>
      <section aria-labelledby="matching-title" className="space-y-3">
        <div className="flex items-center gap-2"><h2 id="matching-title">Matching</h2><FieldHelp label="Matching">Identify the same music, or records describing the same listen.</FieldHelp></div>
        {["music", "play"].map(subtype => <ProgramFilterManager key={subtype} state={saved} config={draft} busy={saving} type="matching" subtype={subtype} embedded onChange={upsert} onRemove={remove} focusId={subtype === "music" && focusedMatching ? focusedFilter : undefined} />)}
      </section>
      <section aria-labelledby="inference-title" className="space-y-3">
        <div className="flex items-center gap-2"><h2 id="inference-title">Inference</h2><FieldHelp label="Inference">Derive missing quantities, dates, or context from available evidence.</FieldHelp></div>
        {saved.sources.some(source => ["itunes", "navidrome"].includes(source.service) && source.records > 0) && <ProgramFilterManager state={saved} config={draft} busy={saving} type="inference" subtype="count" embedded onChange={upsert} onRemove={remove} />}
        <ProgramFilterManager state={saved} config={draft} busy={saving} type="inference" subtype="time" embedded onChange={upsert} onRemove={remove} />
        {(saved.sources.some(source => ["yarg", "setlistfm", "multi-scrobbler"].includes(source.service) && source.records > 0) || draft.rules.some(rule => rule.type === "inference" && rule.subtype === "context")) && <ProgramFilterManager state={saved} config={draft} busy={saving} type="inference" subtype="context" embedded onChange={upsert} onRemove={remove} onMove={move} />}
      </section>
      {saved.sources.some(source => source.records > 0) && <section aria-labelledby="attribution-title" className="space-y-3">
        <div className="flex items-center gap-2"><h2 id="attribution-title">Attribution</h2><FieldHelp label="Attribution">Determine which listening belongs to you.</FieldHelp></div>
        {["direct", "heuristic", "aggregate"].map(subtype => <ProgramFilterManager key={subtype} state={saved} config={draft} busy={saving} type="attribution" subtype={subtype} embedded onChange={upsert} onRemove={remove} />)}
      </section>}
    </>}
    {saved && draft && <section aria-labelledby="qualification-title" className="space-y-3">
      <div className="flex items-center gap-2"><h2 id="qualification-title">Qualification</h2><FieldHelp label="Qualification">Choose which listening meets your counting criteria.</FieldHelp></div>
      <ProgramFilterManager state={saved} config={draft} busy={saving} type="qualification" subtype="completion" embedded onChange={upsert} onRemove={remove} onMove={move} />
    </section>}
    {saved && draft && <section aria-labelledby="presentation-title" className="space-y-3">
      <div className="flex items-center gap-2"><h2 id="presentation-title">Presentation</h2><FieldHelp label="Presentation">Choose how music names and groups appear in the view.</FieldHelp></div>
      {["naming", "grouping"].map(subtype => <ProgramFilterManager key={subtype} state={saved} config={draft} busy={saving} subtype={subtype} embedded onChange={upsert} onRemove={remove} onMove={move} focusId={(subtype === "grouping" ? focusedGrouping : !focusedMatching && !focusedGrouping && !focusedScope) ? focusedFilter : undefined} />)}
    </section>}
    {saved && draft && <section aria-labelledby="inclusion-title" className="space-y-3">
      <div className="flex items-center gap-2"><h2 id="inclusion-title">Inclusion</h2><FieldHelp label="Inclusion">Choose what contributes to the view.</FieldHelp></div>
      <ProgramFilterManager state={saved} config={draft} busy={saving} type="inclusion" subtype="scope" embedded onChange={upsert} onRemove={remove} onMove={move} focusId={focusedScope ? focusedFilter : undefined} />
    </section>}
  </main>;
}


function errorMessage(error: unknown) { return error instanceof Error ? error.message : "Could not save filters."; }
