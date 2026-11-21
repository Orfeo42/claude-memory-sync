import { useEffect, useState } from "react";
import type { JSX } from "react";
import { errorMessage, getFootprint } from "./api";
import { formatBytes, formatTime } from "./format";
import type { Footprint } from "./types";

type FileListProps = {
  title: string;
  bytes: number;
  files: readonly string[];
};

const FileList = ({ title, bytes, files }: FileListProps): JSX.Element => (
  <div>
    <strong>{title}</strong>: {formatBytes(bytes)} ({files.length} files)
    {files.length > 0 && (
      <ul className="plain-list">
        {files.map((file) => (
          <li key={file}>
            <code>{file}</code>
          </li>
        ))}
      </ul>
    )}
  </div>
);

export const FootprintPage = (): JSX.Element => {
  const [footprint, setFootprint] = useState<Footprint | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    const load = async (): Promise<void> => {
      try {
        setFootprint(await getFootprint(controller.signal));
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

  if (error !== null) {
    return (
      <p className="error-text" role="alert">
        {error}
      </p>
    );
  }
  if (footprint === null) {
    return <p className="hint">Loading…</p>;
  }

  const { global, limits, projects } = footprint;

  return (
    <div className="stack">
      <h2>Footprint</h2>
      <small>Generated {formatTime(footprint.generated_at)}</small>
      <section className="group" aria-label="Global">
        <h3>Global</h3>
        <p>
          <strong>CLAUDE.md</strong>: {formatBytes(global.claude_md_bytes)}
        </p>
        <FileList
          title="Unconditional rules"
          bytes={global.rules_unconditional_bytes}
          files={global.rules_unconditional}
        />
        <FileList
          title="Scoped rules"
          bytes={global.rules_scoped_bytes}
          files={global.rules_scoped}
        />
        <p>
          <strong>Skills</strong>: {global.skills}
        </p>
      </section>
      <h3>Projects</h3>
      {projects.length === 0 ? (
        <p className="hint">No projects.</p>
      ) : (
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th scope="col">Project</th>
                <th scope="col">Lines</th>
                <th scope="col">Bytes</th>
                <th scope="col">Entries</th>
                <th scope="col">Usage</th>
              </tr>
            </thead>
            <tbody>
              {projects.map((project) => {
                const warn = project.pct_of_limit >= limits.warn_pct;
                return (
                  <tr
                    key={project.key}
                    className={warn ? "warn" : undefined}
                    data-warn={warn ? "true" : "false"}
                  >
                    <th scope="row">{project.key}</th>
                    <td>
                      {project.memory_lines} / {limits.memory_lines}
                    </td>
                    <td>
                      {project.memory_bytes} / {limits.memory_bytes}
                    </td>
                    <td>{project.entries}</td>
                    <td>
                      <div
                        className="bar"
                        role="progressbar"
                        aria-label={`${project.key} usage`}
                        aria-valuemin={0}
                        aria-valuemax={100}
                        aria-valuenow={Math.min(project.pct_of_limit, 100)}
                      >
                        <div
                          className="bar-fill"
                          style={{ width: `${String(Math.min(project.pct_of_limit, 100))}%` }}
                        />
                      </div>
                      <small>{project.pct_of_limit}%</small>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
};
