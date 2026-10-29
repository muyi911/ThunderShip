import { readFileSync } from "node:fs";

// Component checks replace generated Go calls; desktop smoke checks exercise the real bridge.
export const mockWailsBindings = {
  name: "mock-wails-bindings",
  setup(build) {
    build.onLoad({ filter: /[\\/]bindings[\\/]shell_workflow[\\/]app\.ts$/ }, ({ path }) => {
      const source = readFileSync(path, "utf8");
      const names = [...source.matchAll(/export function (\w+)\(/g)].map(match => match[1]);
      return { contents: names.map(name => `export const ${name} = (...args) => globalThis.backend.${name}(...args);`).join("\n"), loader: "js" };
    });
  },
};
