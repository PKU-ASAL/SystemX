import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      {
        source: '/api/v1/:path*',
        destination: 'http://100.71.164.84:8080/api/v1/:path*',
      },
    ];
  },
};

export default nextConfig;
