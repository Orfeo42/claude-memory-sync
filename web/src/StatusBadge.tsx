import type { JSX } from "react";

type Props = {
  status: string;
};

export const StatusBadge = ({ status }: Props): JSX.Element => (
  <span className={`badge status-${status}`}>{status}</span>
);
