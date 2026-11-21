import { useEffect, useState } from "react";
import type { JSX } from "react";
import { errorMessage, getRun } from "./api";
import { formatTime } from "./format";
import { StatusBadge } from "./StatusBadge";
import type { RunDetail } from "./types";

type Props = {
  id: string;
};

const formatValue = (value: unknown): string =>
  typeof value === "string" ? value : JSON.stringify(value);

export const RunDetailView = ({ id }: Props): JSX.Element => {
  const [detail, setDetail] = useState<RunDetail | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    const load = async (): Promise<void> => {
      try {
        setDetail(await getRun(id, controller.signal));
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
  }, [id]);

  if (error !== null) {
    return (
      <div>
        <a href="#/runs">Back to runs</a>
        <p className="error-text" role="alert">
          {error}
        </p>
      </div>
    );
  }
  if (detail === null) {
    return <p className="hint">Loading…</p>;
  }

  const { run, steps } = detail;
  const summary = Object.entries(run.summary ?? {});

  return (
    <div className="stack">
      <a href="#/runs">Back to runs</a>
      <h2>
        {run.flow} <StatusBadge status={run.status} />
      </h2>
      <dl className="facts">
        <dt>Trigger</dt>
        <dd>{run.trigger}</dd>
        <dt>Started</dt>
        <dd>{formatTime(run.started_at)}</dd>
        <dt>Finished</dt>
        <dd>{formatTime(run.finished_at)}</dd>
      </dl>
      {run.error !== null && run.error !== undefined && (
        <p className="error-text">{run.error}</p>
      )}
      <h3>Summary</h3>
      {summary.length === 0 ? (
        <p className="hint">No summary.</p>
      ) : (
        <dl className="facts">
          {summary.map(([key, value]) => (
            <div key={key} className="fact">
              <dt>{key}</dt>
              <dd>{formatValue(value)}</dd>
            </div>
          ))}
        </dl>
      )}
      <h3>Steps</h3>
      {steps.length === 0 ? (
        <p className="hint">No steps recorded.</p>
      ) : (
        <ul className="steps">
          {steps.map((step) => (
            <li key={step.id}>
              <div className="step-head">
                <strong>{step.step}</strong> <StatusBadge status={step.status} />{" "}
                <small>
                  {formatTime(step.started_at)} → {formatTime(step.finished_at)}
                </small>
              </div>
              {step.error !== null && step.error !== undefined && (
                <p className="error-text">{step.error}</p>
              )}
              {step.output_tail !== null &&
                step.output_tail !== undefined &&
                step.output_tail !== "" && (
                  <details>
                    <summary>Output</summary>
                    <pre>{step.output_tail}</pre>
                  </details>
                )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
};
