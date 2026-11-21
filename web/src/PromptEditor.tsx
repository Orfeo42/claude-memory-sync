import { useEffect, useState } from "react";
import type { JSX } from "react";
import {
  errorMessage,
  getPrompt,
  getPromptHistory,
  resetPrompt,
  savePrompt,
} from "./api";
import { formatTime } from "./format";
import type { Prompt, PromptVersion } from "./types";

type Props = {
  name: string;
};

export const PromptEditor = ({ name }: Props): JSX.Element => {
  const [prompt, setPrompt] = useState<Prompt | null>(null);
  const [history, setHistory] = useState<readonly PromptVersion[]>([]);
  const [draft, setDraft] = useState("");
  const [showDefault, setShowDefault] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    const load = async (): Promise<void> => {
      try {
        const [loaded, versions] = await Promise.all([
          getPrompt(name, controller.signal),
          getPromptHistory(name, controller.signal),
        ]);
        setPrompt(loaded);
        setDraft(loaded.content);
        setHistory(versions);
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
  }, [name]);

  const reloadHistory = async (): Promise<void> => {
    try {
      setHistory(await getPromptHistory(name));
    } catch (historyError: unknown) {
      setError(errorMessage(historyError));
    }
  };

  const save = async (): Promise<void> => {
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      const saved = await savePrompt(name, draft);
      setPrompt(saved);
      setDraft(saved.content);
      setNotice("Saved");
      await reloadHistory();
    } catch (saveError: unknown) {
      setError(errorMessage(saveError));
    }
    setBusy(false);
  };

  const reset = async (): Promise<void> => {
    setBusy(true);
    setError(null);
    setNotice(null);
    setConfirming(false);
    try {
      const restored = await resetPrompt(name);
      setPrompt(restored);
      setDraft(restored.content);
      setNotice("Reset to default");
      await reloadHistory();
    } catch (resetError: unknown) {
      setError(errorMessage(resetError));
    }
    setBusy(false);
  };

  return (
    <div className="stack">
      <a href="#/prompts">Back to prompts</a>
      <h2>
        {name}{" "}
        {prompt?.overridden === true && (
          <span className="badge">overridden</span>
        )}
      </h2>
      {error !== null && (
        <p className="error-text" role="alert">
          {error}
        </p>
      )}
      {notice !== null && <p role="status">{notice}</p>}
      {prompt === null && error === null && <p className="hint">Loading…</p>}
      {prompt !== null && (
        <>
          <textarea
            className="editor"
            aria-label="Prompt content"
            value={draft}
            onChange={(event) => {
              setDraft(event.target.value);
            }}
            rows={18}
            spellCheck={false}
            disabled={busy}
          />
          <div className="actions">
            <button
              type="button"
              className="primary"
              onClick={() => {
                void save();
              }}
              disabled={busy || draft === prompt.content || draft === ""}
            >
              Save
            </button>
            <button
              type="button"
              onClick={() => {
                setConfirming(true);
              }}
              disabled={busy || !prompt.overridden || confirming}
            >
              Reset to default
            </button>
            <button
              type="button"
              aria-pressed={showDefault}
              onClick={() => {
                setShowDefault((current) => !current);
              }}
            >
              Default
            </button>
          </div>
          {confirming && (
            <div className="confirm" role="alertdialog" aria-label="Confirm reset">
              <p>Discard the override and restore the default prompt?</p>
              <div className="actions">
                <button
                  type="button"
                  className="danger"
                  onClick={() => {
                    void reset();
                  }}
                >
                  Confirm reset
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setConfirming(false);
                  }}
                >
                  Cancel
                </button>
              </div>
            </div>
          )}
          {showDefault && (
            <pre className="default-view" aria-label="Default prompt">
              {prompt.default}
            </pre>
          )}
          <h3>History</h3>
          {history.length === 0 ? (
            <p className="hint">No history.</p>
          ) : (
            <ul className="plain-list">
              {history.map((version) => (
                <li key={`${version.created_at}-${version.created_by}`}>
                  <small>
                    {formatTime(version.created_at)} by {version.created_by}
                  </small>
                  {version.deleted || version.content === null ? (
                    <p className="hint">Reset to default</p>
                  ) : (
                    <details>
                      <summary>Content</summary>
                      <pre>{version.content}</pre>
                    </details>
                  )}
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </div>
  );
};
