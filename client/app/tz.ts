export function initTimezoneCookie() {
  if (typeof window === "undefined") return;

  try {
    if (document.cookie.includes("tz=")) return;

    const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
    if (!tz) return;

    document.cookie = `tz=${tz}; Path=/; Max-Age=31536000; SameSite=Lax`;
  } catch {
    // Timezone preferences must not prevent startup when cookies are blocked.
  }
}
