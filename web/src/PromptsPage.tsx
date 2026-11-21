import { useEffect, useState } from "react";
import type { JSX } from "react";
import { errorMessage, listPrompts } from "./api";
import { formatTime } from "./format";
import { PromptEditor } from "./PromptEditor";
import type { PromptSummary } from "./types";

type Props = {
  name: string | null;
};

export const PromptsPage = ({ name }: Props): JSX.Element => {
  if (name !== null) {
    return <PromptEditor key={name} name={name} />;
  }
  return <PromptList />;
};

const PromptList = (): JSX.Element => {
  const [prompts, setPrompts] = useState<readonly PromptSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    const load = async (): Promise<void> => {
      try {
        setPrompts(await listPrompts(controller.signal));
      } catch (loadError: unknown) {
        if (controller.signal.aborted) {
          return;
        }
        setError(errorMessage(loadError));
      }
      setLoading(false);
    };
    void load();
    return () => {
      controller.abort();
    };
  }, []);

  return (
    <div className="stack">
      <h2>Prompts</h2>
      {error !== null && (
        <p className="error-text" role="alert">
          {error}
        </p>
      )}
      {loading && <p className="hint">Loading…</p>}
      {!loading && error === null && prompts.length === 0 && (
        <p className="hint">No prompts.</p>
      )}
      <ul className="plain-list">
        {prompts.map((prompt) => (
          <li key={prompt.name}>
            <a href={`#/prompts/${encodeURIComponent(prompt.name)}`}>
              {prompt.name}
            </a>{" "}
            {prompt.overridden && <span className="badge">overridden</span>}{" "}
            <small>{formatTime(prompt.updated_at)}</small>
          </li>
        ))}
      </ul>
    </div>
  );
};
