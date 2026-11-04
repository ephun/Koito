export type RuleSettings = {
  case_sensitive: boolean;
  match_missing_album: boolean;
  tolerance_seconds: number;
  spotify_time?: string;
  preferred_source?: string;
  timezone: string;
  listener?: string;
  share_percent?: number;
  evidence_source_id?: number;
  minimum_matches?: number;
  count_mode?: string;
  use_last_played?: boolean;
  estimate_times?: boolean;
};

export type Rule = {
	 time_migrated?: boolean;
	 migration?: { program_hash: string; settings_hash: string; scope?: RuleScope };
  priority?: number;
  name?: string;
  program?: FilterProgram;
  id: string;
  type: string;
  subtype: string;
  enabled: boolean;
  scope: RuleScope;
  settings: RuleSettings;
};

export type RuleConfig = { rules: Rule[] };
export type RuleScope = {
  service?: string; source_id?: number; file_hash?: string; library_id?: string; track_id?: string;
  record_id?: number; artist?: string; device?: string; from?: string; until?: string;
};
export type FilterSource = { id: number; service: string; records: number };
export type RuleVersion = { id: number; config: RuleConfig };
export type FilterState = {
  blocks?: { operators: BlockOperator[]; schemas: Record<string, BlockSchema>; legacy?: Record<string, Rule>; additions?: Rule[]; templates?: Record<string, { name: string; label: string; program: FilterProgram }[]> };
  current: RuleVersion;
  time_files: { hash: string; filename: string; source_id: number; records: number }[];
  sources: FilterSource[];
  files: { hash: string; source_id: number; service: string; filename: string }[];
  libraries: { id: string; source_id: number; filename: string }[];
};

export type Expression = { op: string; field?: string; value?: unknown; args?: Expression[] };
export type FilterStatement = { if?: Expression; then?: FilterStatement[]; else_if?: { if: Expression; then: FilterStatement[] }[]; else?: FilterStatement[]; set?: string; value?: Expression };
export type FilterProgram = { steps: FilterStatement[] };
export type BlockOperator = { id: string; label: string; min_args: number; max_args: number };
export type SourceField = { path: string; type: string; label: string };
export type BlockSchema = { help?: Record<string, string>; snapshot_fields?: boolean; item_fields?: Record<string, string>; item_labels?: Record<string, string>; evidence_fields?: boolean; raw_fields?: boolean; labels?: Record<string, string>; fields: Record<string, string>; targets: Record<string, string>; bounds?: Record<string, { min?: number; max?: number; whole?: boolean }>; choices?: Record<string, { label: string; value: string }[]> };

export function sourceName(service: string) {
  return ({ spotify: "Spotify", lastfm: "Last.fm", itunes: "iTunes" } as Record<string, string>)[service] || service;
}

export const defaultRuleSettings: RuleSettings = { case_sensitive: true, match_missing_album: true, tolerance_seconds: 0, timezone: "" };
export type FilterChange = {
  base_version: number;
  config: RuleConfig;
  decision?: { left: number; right: number; action: string };
};

export class FilterRequestError extends Error {
  constructor(message: string, public status: number) { super(message); }
}

export async function filterRequest<T>(path = "", change?: FilterChange): Promise<T> {
  const response = await fetch(`/apis/web/v1/filters${path}`, change === undefined ? undefined : {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(change),
  });
  let data;
  try { data = await response.json(); } catch { throw new Error("Koito returned an invalid response."); }
  if (!response.ok) {
    const message = response.status === 409 ? "Filters changed. Reload and try again."
      : response.status === 401 ? "Sign in to edit filters."
      : response.status >= 500 ? "Could not update the view."
      : data.error || "Could not load data.";
    throw new FilterRequestError(message, response.status);
  }
  return data as T;
}
