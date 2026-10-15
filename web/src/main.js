import "@fontsource-variable/source-serif-4/opsz.css";
import "@fontsource-variable/source-serif-4/opsz-italic.css";
import "@fontsource-variable/instrument-sans/index.css";
import "./styles/app.css";
import "./styles/notes.css";
import { mount } from "svelte";
import App from "./App.svelte";

mount(App, { target: document.body });
