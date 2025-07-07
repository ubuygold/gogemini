import { defineConfig } from "vite";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import fs from "fs";
import yaml from "js-yaml";

// Load and parse the YAML configuration file
const config = yaml.load(fs.readFileSync("../config.yaml", "utf8")) as {
  port: number;
};
const apiPort = config.port || 8080;

export default defineConfig({
  plugins: [tailwindcss(), react()],
  build: {
    outDir: "../cmd/gogemini/dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/admin": `http://localhost:${apiPort}`,
      "/gemini": `http://localhost:${apiPort}`,
      "/openai": `http://localhost:${apiPort}`,
    },
  },
});