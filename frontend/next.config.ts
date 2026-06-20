import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Produce a self-contained server bundle (.next/standalone) for a small Docker image.
  output: "standalone",
};

export default nextConfig;
