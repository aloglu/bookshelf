import "@fontsource-variable/fraunces/opsz.css";
import "@fontsource-variable/fraunces/opsz-italic.css";
import "@fontsource-variable/instrument-sans/index.css";
import "./styles/app.css";
import { mount } from "svelte";
import App from "./App.svelte";

mount(App, { target: document.body });
