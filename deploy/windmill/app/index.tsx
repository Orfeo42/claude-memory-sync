import * as React from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { startThemeSync } from "./theme";
import "./index.css";

startThemeSync();
const container = document.getElementById("root");
if (container === null) {
  throw new Error("root element not found");
}
createRoot(container).render(<App />);
