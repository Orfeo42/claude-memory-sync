export const formatTime = (iso: string | null | undefined): string =>
  iso === null || iso === undefined || iso === ""
    ? "—"
    : new Date(iso).toLocaleString();

export const formatBytes = (bytes: number): string =>
  bytes < 1024 ? `${String(bytes)} B` : `${(bytes / 1024).toFixed(1)} KiB`;
