import type { NextConfig } from "next";

const isGitHubPages = process.env.GITHUB_ACTIONS === "true";

const nextConfig: NextConfig = {
    output: "export",
    trailingSlash: true,
    basePath: isGitHubPages ? "/sta-tw.org" : "",
    images: {
        unoptimized: true
    }
};

export default nextConfig;
