import { useEffect, useState } from "react";

export const TABS = [
  "proposals",
  "runs",
  "prompts",
  "settings",
  "footprint",
] as const;

export type Tab = (typeof TABS)[number];

export type Route = {
  tab: Tab;
  id: string | null;
};

const isTab = (value: string): value is Tab =>
  TABS.some((tab) => tab === value);

export const parseRoute = (hash: string): Route => {
  const [first = "", ...rest] = hash.replace(/^#\/?/, "").split("/");
  const tab: Tab = isTab(first) ? first : "proposals";
  const raw = rest.join("/");
  if (raw === "") {
    return { tab, id: null };
  }
  try {
    return { tab, id: decodeURIComponent(raw) };
  } catch {
    return { tab, id: null };
  }
};

export const useRoute = (): Route => {
  const [hash, setHash] = useState(window.location.hash);

  useEffect(() => {
    const onChange = (): void => {
      setHash(window.location.hash);
    };
    window.addEventListener("hashchange", onChange);
    return () => {
      window.removeEventListener("hashchange", onChange);
    };
  }, []);

  return parseRoute(hash);
};
