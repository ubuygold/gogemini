import { defineConfig } from "vite";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import fs from "fs";
import yaml from "js-yaml";

// Define a function to get the API port
const getApiPort = () => {
  // In development, try to read from env var first, then config file.
  if (process.env.GOGEMINI_PORT) {
    return parseInt(process.env.GOGEMINI_PORT, 10);
  }

  try {
    const configFile = fs.readFileSync("../config.yaml", "utf8");
    const config = yaml.load(configFile) as { port: number };
    return config.port || 8081;
  } catch (error) {
    console.warn("Could not read config.yaml, using default port 8081.", error);
    return 8081;
  }
};

const apiPort = getApiPort();

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
