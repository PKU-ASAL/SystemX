import type { NextConfig } from "next";

import { getManagerApiOrigin } from "./lib/api/manager-origin";

const managerApiOrigin = getManagerApiOrigin();

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      {
        source: "/api/v1/:path*",
        destination: `${managerApiOrigin}/api/v1/:path*`,
      },
    ];
  },
};

export default nextConfig;
