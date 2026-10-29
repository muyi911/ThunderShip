import { createApp } from "vue";
import { createPinia } from "pinia";
import { createVuetify } from "vuetify";
import * as components from "vuetify/components";
import * as directives from "vuetify/directives";
import "vuetify/styles";
import "@mdi/font/css/materialdesignicons.css";
import "./style.css";
import App from "./App.vue";
const vuetify = createVuetify({
  components,
  directives,
  defaults: {
    VTextField: {
      variant: "outlined",
      density: "compact",
      hideDetails: "auto",
    },
    VSelect: { variant: "outlined", density: "compact", hideDetails: "auto" },
    VCombobox: { variant: "outlined", density: "compact", hideDetails: "auto" },
    VBtn: { variant: "text", rounded: "sm" },
    VCheckbox: { density: "compact", hideDetails: true },
  },
  theme: {
    defaultTheme: matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light",
    themes: {
      light: {
        colors: {
          primary: "#3671E8",
          background: "#EDEFF2",
          surface: "#FFFFFF",
          error: "#d84315",
          success: "#388657",
        },
      },
      dark: {
        dark: true,
        colors: {
          primary: "#82B1FF",
          background: "#1E1E1E",
          surface: "#222222",
          error: "#ea5252",
          success: "#76b03d",
        },
      },
    },
  },
});
createApp(App).use(createPinia()).use(vuetify).mount("#app");
