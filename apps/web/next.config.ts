import type { NextConfig } from "next";
import { resolve } from "node:path";

const nextConfig: NextConfig = {
  poweredByHeader: false,
  // Use canonical monorepo contracts rather than an app-local duplicate.
  turbopack: { root: resolve(__dirname, "../..") },
};

export default nextConfig;
