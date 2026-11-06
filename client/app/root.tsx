import {
  isRouteErrorResponse,
  Links,
  Meta,
  Outlet,
  Scripts,
  ScrollRestoration,
  useRouteError,
  useNavigation,
} from "react-router";

import type { Route } from "./+types/root";
import "./themes.css";
import "./app.css";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ThemeProvider } from "./providers/ThemeProvider";
import Sidebar from "./components/sidebar/Sidebar";
import Footer from "./components/Footer";
import { AppProvider } from "./providers/AppProvider";
import { initTimezoneCookie } from "./tz";
import Loading from "./components/Loading";

initTimezoneCookie();

// Create a client
const queryClient = new QueryClient();

export const links: Route.LinksFunction = () => [
  { rel: "preconnect", href: "https://fonts.googleapis.com" },
  {
    rel: "preconnect",
    href: "https://fonts.gstatic.com",
    crossOrigin: "anonymous",
  },
  {
    rel: "stylesheet",
    href: "https://fonts.googleapis.com/css2?family=Inter:ital,opsz,wght@0,14..32,100..900;1,14..32,100..900&display=swap",
  },
];

export function Layout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <head>
        <script
          dangerouslySetInnerHTML={{
            __html: `
              (function loadtheme() {
                try {
                  var bg = localStorage.getItem("bgcolor");
                  document.documentElement.style.backgroundColor = bg;
                  var theme = localStorage.getItem("theme");
                  if (theme) document.documentElement.setAttribute("data-theme", theme);
                  if (theme === "custom") {
                    var custom = JSON.parse(localStorage.getItem("custom-theme") || "null");
                    if (custom && typeof custom.bg === "string" && typeof custom.fg === "string") {
                      document.documentElement.style.setProperty("--color-bg", custom.bg);
                      document.documentElement.style.setProperty("--color-fg", custom.fg);
                    }
                  }
                } catch (e) {
                  console.log(e);
                }
              })();
            `,
          }}
        />
        <meta charSet="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <link
          rel="icon"
          type="image/png"
          href="/favicon-96x96.png"
          sizes="96x96"
        />
        <link rel="icon" type="image/svg+xml" href="/favicon.svg" />
        <link rel="shortcut icon" href="/favicon.ico" />
        <link
          rel="apple-touch-icon"
          sizes="180x180"
          href="/apple-touch-icon.png"
        />
        <meta name="apple-mobile-web-app-title" content="Koito" />
        <link rel="manifest" href="/site.webmanifest" />
        <Meta />
        <Links />
      </head>
      <body className="min-h-screen">
        {children}
        <ScrollRestoration />
        <Scripts />
      </body>
    </html>
  );
}

export default function App() {
  const navigation = useNavigation();
  const navigating = navigation.state !== "idle";
  return (
    <>
      {navigating && <div role="status" aria-label="Loading page" className="fixed inset-x-0 top-0 z-[100] h-1 animate-pulse bg-(--color-fg-secondary)"><span className="sr-only">Loading page</span></div>}
      <AppProvider>
        <ThemeProvider>
          <QueryClientProvider client={queryClient}>
            <div className="flex-col flex sm:flex-row">
              <Sidebar />
              <div aria-busy={navigating} className="flex flex-col items-center mx-auto w-full ml-0 sm:ml-[58px]">
                <Outlet />
                <Footer />
              </div>
            </div>
          </QueryClientProvider>
        </ThemeProvider>
      </AppProvider>
    </>
  );
}

export function HydrateFallback() {
  return <Loading />;
}

export function ErrorBoundary() {
  const error = useRouteError();
  let message = "Oops!";
  let details = "An unexpected error occurred.";
  let stack: string | undefined;

  if (isRouteErrorResponse(error)) {
    message = error.status === 404 ? "404" : "Error";
    details =
      error.status === 404
        ? "The requested page could not be found."
        : error.statusText || details;
  } else if (import.meta.env.DEV && error instanceof Error) {
    details = error.message;
    stack = error.stack;
  }

  const title = `${message} - Koito`;

  return (
    <div style={{ minHeight: "100vh", padding: "2rem", background: "#191310", color: "#f5ece3" }}>
        <title>{title}</title>
        <div className="flex">
          <div className="w-full flex flex-col">
            <main className="pt-16 p-4 w-4/5 min-w-[350px] mx-auto flex-grow">
              <div className="md:flex gap-4">
                <img className="w-[200px] rounded mb-3" src="../yuu.jpg" />
                <div>
                  <h1>{message}</h1>
                  <p>{details}</p>
                </div>
              </div>
              {stack && (
                <pre className="w-full p-4 overflow-x-auto">
                  <code>{stack}</code>
                </pre>
              )}
            </main>
          </div>
        </div>
      <button type="button" className="underline" onClick={() => window.location.reload()}>Reload</button>
    </div>
  );
}
