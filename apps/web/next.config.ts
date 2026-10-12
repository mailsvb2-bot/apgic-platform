import type { NextConfig } from "next";
import { resolve } from "node:path";

const nextConfig: NextConfig = {
  poweredByHeader: false,
  // Web and native clients consume one canonical contract outside apps/web.
  // Turbopack must see the monorepo root, not an app-local lockfile boundary.
  turbopack: { root: resolve(__dirname, "../..") },
};

export default nextConfig;
