import { login } from "api/api";
import { useState, type FormEvent } from "react";
import SubHeader from "../primitives/SubHeader";

export default function LoginForm() {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function loginHandler(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (loading) return;
    // Read actual inputs, including values supplied by browser autofill.
    const data = new FormData(event.currentTarget);
    const username = String(data.get("koito-username") || "").trim();
    const password = String(data.get("koito-password") || "");
    setError("");
    if (!username || !password) {
      setError("Username and password are required.");
      return;
    }

    setLoading(true);
    const controller = new AbortController();
    const timeout = window.setTimeout(() => controller.abort(), 10000);
    try {
      const response = await login(username, password, data.has("koito-remember"), controller.signal);
      if (!response.ok) {
        if (response.status === 429) setError("Too many login attempts. Try again in a minute.");
        else if (response.status === 401) setError("Incorrect username or password.");
        else setError(`Login failed (${response.status}). Try again.`);
        return;
      }
      const session = await fetch("/apis/web/v1/user", { credentials: "same-origin", cache: "no-store", signal: controller.signal });
      if (session.status === 401) {
        setError("Your browser did not keep the login session. Allow cookies for this site.");
        return;
      }
      if (!session.ok) {
        setError("Could not load your login session. Try again.");
        return;
      }
      window.location.reload();
    } catch (error) {
      setError(error instanceof Error && error.name === "AbortError"
        ? "Login timed out. Try again."
        : "Could not reach Koito. Try again.");
    } finally {
      window.clearTimeout(timeout);
      setLoading(false);
    }
  }

  return (
    <>
      <SubHeader>Log In</SubHeader>
      <div className="flex flex-col items-center gap-4 w-full">
        <form
          className="flex flex-col items-center gap-4 w-3/4"
          onSubmit={loginHandler}
          noValidate
        >
          <input
            name="koito-username"
            type="text"
            autoComplete="username"
            aria-label="Username"
            placeholder="Username"
            className="w-full mx-auto fg bg rounded p-2"
            disabled={loading}
          />
          <input
            name="koito-password"
            type="password"
            autoComplete="current-password"
            aria-label="Password"
            placeholder="Password"
            className="w-full mx-auto fg bg rounded p-2"
            disabled={loading}
          />
          <div className="flex gap-2 items-center">
            <input
              type="checkbox"
              name="koito-remember"
              id="koito-remember"
              disabled={loading}
            />
            <label htmlFor="koito-remember">Remember me</label>
          </div>
          <button type="submit" className="large-button disabled:opacity-50" disabled={loading}>
            {loading ? "Logging in…" : "Login"}
          </button>
        </form>
        {error && <p role="alert" className="text-(--color-error)">{error}</p>}
      </div>
    </>
  );
}
