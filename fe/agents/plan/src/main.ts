import { mount } from "svelte";
import App from "./App.svelte";

const target = document.getElementById("app");
if (target) {
  // The Go shell renders #app without data-base on the plan page (the SPA
  // knows its mount), so fall back to the route the bundle was served from.
  if (!target.dataset.base) target.dataset.base = "/tools/agents";
  mount(App, { target });
}
