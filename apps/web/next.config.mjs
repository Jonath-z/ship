const development = process.env.NODE_ENV === "development";

/** @type {import('next').NextConfig} */
const nextConfig = {
  distDir: process.env.NEXT_DIST_DIR ?? ".next",
  // Production is a static export served by ship-api; next dev keeps a
  // server, so it proxies /api to the Go API instead.
  ...(development
    ? {
        rewrites: async () => [
          {
            source: "/api/:path*",
            destination: `${process.env.SHIP_API_URL ?? "http://localhost:8080"}/api/:path*`,
          },
        ],
      }
    : { output: "export" }),
  transpilePackages: ["@ship/api-client", "@ship/types", "@ship/ui"],
};

export default nextConfig;
