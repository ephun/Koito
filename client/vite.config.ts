import { reactRouter } from "@react-router/dev/vite";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";
import tsconfigPaths from "vite-tsconfig-paths";
import { vanillaExtractPlugin } from "@vanilla-extract/vite-plugin";

const isDocker = process.env.BUILD_TARGET === "docker";
const backendTarget = process.env.KOITO_DEV_API || "http://127.0.0.1:4110";

export default defineConfig({
  plugins: [tailwindcss(), reactRouter(), vanillaExtractPlugin()],
  server: {
    proxy: {
      "/apis": {
        target: backendTarget,
        changeOrigin: true,
      },
      "/image": {
        target: backendTarget,
        changeOrigin: true,
      },
    },
  },
  resolve: {
    tsconfigPaths: true,
    alias: {
      ...(isDocker ? { "react-dom/server": "react-dom/server.node" } : {}),
    },
  },
});
