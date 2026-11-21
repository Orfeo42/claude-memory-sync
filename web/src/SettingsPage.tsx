import { useCallback, useEffect, useState } from "react";
import type { JSX } from "react";
import { errorMessage, getSettings } from "./api";
import { ParamRow } from "./ParamRow";
import { TokenForm } from "./TokenForm";
import type { Param, Settings, TokenStatus } from "./types";

export const SettingsPage = (): JSX.Element => {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    const load = async (): Promise<void> => {
      try {
        setSettings(await getSettings(controller.signal));
      } catch (loadError: unknown) {
        if (!controller.signal.aborted) {
          setError(errorMessage(loadError));
        }
      }
    };
    void load();
    return () => {
      controller.abort();
    };
  }, []);

  const onParamChanged = useCallback((param: Param): void => {
    setSettings((current) =>
      current === null
        ? current
        : {
            ...current,
            params: current.params.map((p) => (p.key === param.key ? param : p)),
          },
    );
  }, []);

  const onTokenChanged = useCallback((token: TokenStatus): void => {
    setSettings((current) => (current === null ? current : { ...current, token }));
  }, []);

  return (
    <div className="stack">
      <h2>Settings</h2>
      {error !== null && (
        <p className="error-text" role="alert">
          {error}
        </p>
      )}
      {settings === null && error === null && <p className="hint">Loading…</p>}
      {settings !== null && (
        <>
          {settings.params.length === 0 && (
            <p className="hint">No parameters.</p>
          )}
          <ul className="plain-list">
            {settings.params.map((param) => (
              <ParamRow
                key={param.key}
                param={param}
                onChanged={onParamChanged}
              />
            ))}
          </ul>
          <TokenForm token={settings.token} onChanged={onTokenChanged} />
        </>
      )}
    </div>
  );
};
