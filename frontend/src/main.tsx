// 브라우저에 ModuDock React 애플리케이션을 마운트한다.
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "@tabler/core/dist/css/tabler.min.css";
import "./styles.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
