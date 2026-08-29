import path from "node:path";
import { fileURLToPath } from "node:url";

const currentDirectory = path.dirname(fileURLToPath(import.meta.url));

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  outputFileTracingRoot: path.join(currentDirectory, "../.."),
  transpilePackages: ["@ship/api-client", "@ship/types", "@ship/ui"],
};

export default nextConfig;
