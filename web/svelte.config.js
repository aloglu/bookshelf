// The cards, sheets and drawer are sections, forms and asides with role="dialog", which is valid ARIA;
// Svelte's check only expects that role on a div.
export default {
  compilerOptions: {
    warningFilter: (w) => w.code !== "a11y_no_noninteractive_element_to_interactive_role",
  },
};
