import { ViteSSG } from "vite-ssg";
import App from "./App.vue";
import { routes, scrollBehavior } from "./router";
import "./styles.css";

if (typeof document !== "undefined") {
  document.documentElement.classList.add("js");
}

export const createApp = ViteSSG(App, { routes, scrollBehavior });
