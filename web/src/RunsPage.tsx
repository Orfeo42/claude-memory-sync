import { useCallback, useEffect, useState } from "react";
import type { JSX } from "react";
import { ApiError, errorMessage, listFlows, listRuns, runFlow } from "./api";
import { FlowRow } from "./FlowRow";
import { formatTime } from "./format";
import { RunDetailView } from "./RunDetailView";
import { StatusBadge } from "./StatusBadge";
import type { Flow, Run } from "./types";

type Props = {
  id: string | null;
};

const PAGE_SIZE = 50;
const REFRESH_MS = 10000;
const RUN_STATUSES = [
  "running",
  "succeeded",
  "failed",
  "partial",
  "interrupted",
] as const;

export const RunsPage = ({ id }: Props): JSX.Element => {
  if (id !== null) {
    return <RunDetailView id={id} />;
  }
  return <RunsOverview />;
};

const RunsOverview = (): JSX.Element => {
  const [flows, setFlows] = useState<readonly Flow[]>([]);
  const [flowsLoading, setFlowsLoading] = useState(true);
  const [flowsError, setFlowsError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [runs, setRuns] = useState<readonly Run[]>([]);
  const [runsLoading, setRunsLoading] = useState(true);
  const [runsError, setRunsError] = useState<string | null>(null);
  const [moreBusy, setMoreBusy] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const [flowFilter, setFlowFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");

  const loadFlows = useCallback(async (signal?: AbortSignal): Promise<void> => {
    try {
      setFlows(await listFlows(signal));
      setFlowsError(null);
    } catch (loadError: unknown) {
      if (signal?.aborted === true) {
        return;
      }
      setFlowsError(errorMessage(loadError));
    }
    setFlowsLoading(false);
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    const start = async (): Promise<void> => {
      await loadFlows(controller.signal);
    };
    void start();
    const timer = window.setInterval(() => {
      if (document.visibilityState === "visible") {
        void loadFlows(controller.signal);
      }
    }, REFRESH_MS);
    return () => {
      controller.abort();
      window.clearInterval(timer);
    };
  }, [loadFlows]);

  useEffect(() => {
    const controller = new AbortController();
    const load = async (): Promise<void> => {
      try {
        const page = await listRuns(
          {
            limit: PAGE_SIZE,
            ...(flowFilter === "" ? {} : { flow: flowFilter }),
            ...(statusFilter === "" ? {} : { status: statusFilter }),
          },
          controller.signal,
        );
        setRuns(page);
        setHasMore(page.length === PAGE_SIZE);
        setRunsError(null);
      } catch (loadError: unknown) {
        if (controller.signal.aborted) {
          return;
        }
        setRunsError(errorMessage(loadError));
      }
      setRunsLoading(false);
    };
    void load();
    return () => {
      controller.abort();
    };
  }, [flowFilter, statusFilter]);

  const loadMore = async (): Promise<void> => {
    const last = runs[runs.length - 1];
    if (last === undefined) {
      return;
    }
    setMoreBusy(true);
    try {
      const page = await listRuns({
        limit: PAGE_SIZE,
        before: last.started_at,
        ...(flowFilter === "" ? {} : { flow: flowFilter }),
        ...(statusFilter === "" ? {} : { status: statusFilter }),
      });
      setRuns((current) => [...current, ...page]);
      setHasMore(page.length === PAGE_SIZE);
    } catch (moreError: unknown) {
      setRunsError(errorMessage(moreError));
    }
    setMoreBusy(false);
  };

  const run = useCallback(
    async (name: string): Promise<void> => {
      setMessage(null);
      try {
        await runFlow(name);
        setMessage(`${name} started`);
      } catch (runError: unknown) {
        setMessage(
          runError instanceof ApiError && runError.status === 409
            ? `${name} is already running`
            : errorMessage(runError),
        );
      }
      await loadFlows();
    },
    [loadFlows],
  );

  const onRun = useCallback(
    (name: string): void => {
      void run(name);
    },
    [run],
  );

  const onChanged = useCallback((flow: Flow): void => {
    setFlows((current) => current.map((f) => (f.name === flow.name ? flow : f)));
  }, []);

  return (
    <div className="stack">
      <h2>Flows</h2>
      {flowsError !== null && (
        <p className="error-text" role="alert">
          {flowsError}
        </p>
      )}
      {message !== null && <p role="status">{message}</p>}
      {flowsLoading && <p className="hint">Loading…</p>}
      {!flowsLoading && flows.length === 0 && (
        <p className="hint">No flows.</p>
      )}
      {flows.length > 0 && (
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th scope="col">Flow</th>
                <th scope="col">Schedule</th>
                <th scope="col">State</th>
                <th scope="col">Last run</th>
                <th scope="col">Action</th>
              </tr>
            </thead>
            <tbody>
              {flows.map((flow) => (
                <FlowRow
                  key={flow.name}
                  flow={flow}
                  onRun={onRun}
                  onChanged={onChanged}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
      <h2>Runs</h2>
      <div className="inline-form">
        <label>
          Flow{" "}
          <select
            value={flowFilter}
            onChange={(event) => {
              setRunsLoading(true);
              setFlowFilter(event.target.value);
            }}
          >
            <option value="">all</option>
            {flows.map((flow) => (
              <option key={flow.name} value={flow.name}>
                {flow.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Status{" "}
          <select
            value={statusFilter}
            onChange={(event) => {
              setRunsLoading(true);
              setStatusFilter(event.target.value);
            }}
          >
            <option value="">all</option>
            {RUN_STATUSES.map((status) => (
              <option key={status} value={status}>
                {status}
              </option>
            ))}
          </select>
        </label>
      </div>
      {runsError !== null && (
        <p className="error-text" role="alert">
          {runsError}
        </p>
      )}
      {runsLoading && <p className="hint">Loading…</p>}
      {!runsLoading && runs.length === 0 && (
        <p className="hint">No runs found.</p>
      )}
      {runs.length > 0 && (
        <ul className="plain-list">
          {runs.map((item) => (
            <li key={item.id}>
              <a href={`#/runs/${encodeURIComponent(item.id)}`}>
                {item.flow} <StatusBadge status={item.status} />{" "}
                <small>
                  {item.trigger} — {formatTime(item.started_at)}
                </small>
              </a>
            </li>
          ))}
        </ul>
      )}
      {hasMore && (
        <div>
          <button
            type="button"
            onClick={() => {
              void loadMore();
            }}
            disabled={moreBusy}
          >
            Load more
          </button>
        </div>
      )}
    </div>
  );
};
