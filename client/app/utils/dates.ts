export function calendarDate(value: string) {
  return new Date(`${value}T00:00:00Z`).toLocaleDateString(undefined, { timeZone: "UTC" });
}
