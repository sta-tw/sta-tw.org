import type { NextConfig } from "next";

const nextConfig: NextConfig = {
    output: "standalone",
    trailingSlash: true,
    basePath: process.env.NEXT_BASE_PATH || undefined,
    images: {
        unoptimized: true
    }
};

export default nextConfig;
