import { getCfg, type User } from "api/api";
import { createContext, useContext, useEffect, useState } from "react";
import pkg from "../../package.json";
import semver from "semver";
import { themes } from "~/styles/themes.css";
import Loading from "~/components/Loading";

function isNewerVersion(current: string, latest: string): boolean {
  return !!semver.valid(current) && !!semver.valid(latest) && semver.gt(latest, current);
}

interface AppContextType {
  user: User | null | undefined;
  configurableHomeActivity: boolean;
  homeItems: number;
  defaultTheme: string;
  currentVersion: string;
  updateAvailable: boolean;
  firstActivity: Date | undefined;
  setConfigurableHomeActivity: (value: boolean) => void;
  setHomeItems: (value: number) => void;
  setUsername: (value: string) => void;
}

const AppContext = createContext<AppContextType | undefined>(undefined);

export const useAppContext = () => {
  const context = useContext(AppContext);
  if (context === undefined) {
    throw new Error("useAppContext must be used within an AppProvider");
  }
  return context;
};

export const AppProvider = ({ children }: { children: React.ReactNode }) => {
  const [user, setUser] = useState<User | null | undefined>(undefined);
  const [defaultTheme, setDefaultTheme] = useState("yuu");
  const [connectionError, setConnectionError] = useState(false);
  const [configurableHomeActivity, setConfigurableHomeActivity] =
    useState<boolean>(false);
  const [homeItems, setHomeItems] = useState<number>(0);

  const setUsername = (value: string) => {
    if (!user) {
      return;
    }
    setUser({ ...user, username: value });
  };

  const currentVersion = import.meta.env.VITE_KOITO_VERSION || pkg.version;

  const [updateAvailable, setUpdateAvailable] = useState<boolean>(false);
  const [firstActivity, setFirstActivity] = useState<Date | undefined>();

  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    const timeout = window.setTimeout(() => controller.abort(), 10000);
    fetch("/apis/web/v1/user", { signal: controller.signal })
      .then(async res => {
        if (res.status === 401) return null;
        if (!res.ok) throw new Error("User request failed");
        return res.json();
      })
      .then((data) => {
        if (active) setUser(data?.error ? null : data);
      })
      .catch(() => { if (active) { setUser(null); setConnectionError(true); } })
      .finally(() => window.clearTimeout(timeout));

    setConfigurableHomeActivity(true);
    setHomeItems(12);

    getCfg().then((cfg) => {
      if (active && Object.hasOwn(themes, cfg.default_theme)) {
        setDefaultTheme(cfg.default_theme);
      }
    }).catch(() => { /* Keep the default theme when config is unavailable. */ });

    fetch("/apis/web/v1/first-activity")
      .then((r) => r.json())
      .then((data) => {
        if (!data.error) {
          setFirstActivity(new Date(data.time));
        }
      })
      .catch(() => {});
    return () => { active = false; window.clearTimeout(timeout); controller.abort(); };
  }, []);

  useEffect(() => {
    fetch("https://api.github.com/repos/gabehf/koito/releases/latest")
      .then((r) => r.json())
      .then((r) => {
        setUpdateAvailable(isNewerVersion(currentVersion, r.tag_name));
      })
      .catch(() => {});
  }, []);

  if (user === undefined) {
    return <Loading />;
  }

  const contextValue: AppContextType = {
    user,
    configurableHomeActivity,
    homeItems,
    defaultTheme,
    currentVersion,
    updateAvailable,
    firstActivity,
    setConfigurableHomeActivity,
    setHomeItems,
    setUsername,
  };

  return (
    <AppContext.Provider value={contextValue}>
      {connectionError && <div role="alert" style={{ padding: "1rem", background: "#191310", color: "#f5ece3" }}>Could not connect to Koito. <button type="button" className="underline" onClick={() => window.location.reload()}>Retry</button></div>}
      {children}
    </AppContext.Provider>
  );
};
