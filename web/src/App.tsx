import type { JSX } from "react";
import { FootprintPage } from "./FootprintPage";
import { PromptsPage } from "./PromptsPage";
import { ProposalsPage } from "./ProposalsPage";
import { RunsPage } from "./RunsPage";
import { SettingsPage } from "./SettingsPage";
import { TABS, useRoute } from "./route";
import type { Route } from "./route";

const LABELS = {
  proposals: "Proposals",
  runs: "Runs",
  prompts: "Prompts",
  settings: "Settings",
  footprint: "Footprint",
} as const;

type PageProps = {
  route: Route;
};

const Page = ({ route }: PageProps): JSX.Element => {
  switch (route.tab) {
    case "proposals":
      return <ProposalsPage />;
    case "runs":
      return <RunsPage id={route.id} />;
    case "prompts":
      return <PromptsPage name={route.id} />;
    case "settings":
      return <SettingsPage />;
    case "footprint":
      return <FootprintPage />;
  }
};

export const App = (): JSX.Element => {
  const route = useRoute();

  return (
    <div className="app">
      <nav className="topnav" aria-label="Sections">
        <strong className="brand">Synth</strong>
        {TABS.map((tab) => (
          <a
            key={tab}
            href={`#/${tab}`}
            aria-current={route.tab === tab ? "page" : undefined}
          >
            {LABELS[tab]}
          </a>
        ))}
      </nav>
      <main className="content">
        <Page route={route} />
      </main>
    </div>
  );
};
