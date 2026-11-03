export type RuleSettings = {
  case_sensitive: boolean;
  match_missing_album: boolean;
  tolerance_seconds: number;
  spotify_time?: string;
  preferred_source?: string;
  timezone: string;
};

export type Rule = {
  id: string;
  type: string;
  subtype: string;
  enabled: boolean;
  scope: { service?: string; source_id?: number; file_hash?: string };
  settings: RuleSettings;
};

export type RuleConfig = { rules: Rule[] };
export type RuleVersion = { id: number; config: RuleConfig };
export type FilterState = {
  current: RuleVersion;
  time_files: { hash: string; filename: string; source_id: number; records: number }[];
};
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
    const message = response.status === 409 ? "Rules changed. Reload and try again."
      : response.status === 401 ? "Sign in to edit rules."
      : response.status >= 500 ? "Could not update the view."
      : data.error || "Could not load data.";
    throw new FilterRequestError(message, response.status);
  }
  return data as T;
}
