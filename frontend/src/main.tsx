import { RegistryProvider } from "@effect/atom-react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import "./styles/style.css";

const rootElement = document.getElementById("root");
if (!rootElement) {
  throw new Error("Missing #root element");
}

createRoot(rootElement).render(
  <RegistryProvider>
    <App />
  </RegistryProvider>,
);
