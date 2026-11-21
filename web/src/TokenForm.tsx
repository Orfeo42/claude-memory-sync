import { useState } from "react";
import type { JSX } from "react";
import { clearToken, errorMessage, saveToken } from "./api";
import { formatTime } from "./format";
import type { TokenStatus } from "./types";

type Props = {
  token: TokenStatus;
  onChanged: (token: TokenStatus) => void;
};

export const TokenForm = ({ token, onChanged }: Props): JSX.Element => {
  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const save = async (): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      onChanged(await saveToken(input.trim()));
      setInput("");
    } catch (saveError: unknown) {
      setError(errorMessage(saveError));
    }
    setBusy(false);
  };

  const clear = async (): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      onChanged(await clearToken());
    } catch (clearError: unknown) {
      setError(errorMessage(clearError));
    }
    setBusy(false);
  };

  return (
    <div className="stack">
      <h3>Claude OAuth token</h3>
      <p role="status">
        {token.set
          ? `Token is set (updated ${formatTime(token.updated_at)})`
          : "Token is not set"}
      </p>
      <div className="inline-form">
        <input
          type="password"
          aria-label="New token"
          autoComplete="off"
          value={input}
          onChange={(event) => {
            setInput(event.target.value);
          }}
          disabled={busy}
        />
        <button
          type="button"
          className="primary"
          onClick={() => {
            void save();
          }}
          disabled={busy || input.trim() === ""}
        >
          Save token
        </button>
        <button
          type="button"
          className="danger"
          onClick={() => {
            void clear();
          }}
          disabled={busy || !token.set}
        >
          Clear token
        </button>
      </div>
      {error !== null && (
        <p className="error-text" role="alert">
          {error}
        </p>
      )}
    </div>
  );
};
