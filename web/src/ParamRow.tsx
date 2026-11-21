import { useState } from "react";
import type { JSX } from "react";
import { errorMessage, resetParam, saveParam } from "./api";
import { MODELS } from "./types";
import type { Param } from "./types";

type Props = {
  param: Param;
  onChanged: (param: Param) => void;
};

type ControlProps = {
  param: Param;
  draft: string;
  disabled: boolean;
  onChange: (value: string) => void;
};

const Control = ({
  param,
  draft,
  disabled,
  onChange,
}: ControlProps): JSX.Element => {
  if (param.kind === "model") {
    return (
      <select
        aria-label={param.key}
        value={draft}
        disabled={disabled}
        onChange={(event) => {
          onChange(event.target.value);
        }}
      >
        {MODELS.map((model) => (
          <option key={model} value={model}>
            {model}
          </option>
        ))}
      </select>
    );
  }
  if (param.kind === "bool") {
    return (
      <input
        type="checkbox"
        aria-label={param.key}
        checked={draft === "true"}
        disabled={disabled}
        onChange={(event) => {
          onChange(event.target.checked ? "true" : "false");
        }}
      />
    );
  }
  return (
    <input
      type="number"
      aria-label={param.key}
      value={draft}
      disabled={disabled}
      onChange={(event) => {
        onChange(event.target.value);
      }}
    />
  );
};

export const ParamRow = ({ param, onChanged }: Props): JSX.Element => {
  const [draft, setDraft] = useState(param.value);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const save = async (): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      onChanged(await saveParam(param.key, draft));
    } catch (saveError: unknown) {
      setError(errorMessage(saveError));
    }
    setBusy(false);
  };

  const reset = async (): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      const restored = await resetParam(param.key);
      setDraft(restored.value);
      onChanged(restored);
    } catch (resetError: unknown) {
      setError(errorMessage(resetError));
    }
    setBusy(false);
  };

  return (
    <li className="param">
      <div className="param-head">
        <strong>{param.key}</strong>{" "}
        {param.overridden && <span className="badge">overridden</span>}
      </div>
      <small>
        {param.description} (default: <code>{param.default}</code>)
      </small>
      <div className="inline-form">
        <Control
          param={param}
          draft={draft}
          disabled={busy}
          onChange={setDraft}
        />
        <button
          type="button"
          className="primary"
          aria-label={`Save ${param.key}`}
          onClick={() => {
            void save();
          }}
          disabled={busy || draft === param.value}
        >
          Save
        </button>
        <button
          type="button"
          aria-label={`Reset ${param.key}`}
          onClick={() => {
            void reset();
          }}
          disabled={busy || !param.overridden}
        >
          Reset
        </button>
      </div>
      {error !== null && (
        <p className="error-text" role="alert">
          {error}
        </p>
      )}
    </li>
  );
};
