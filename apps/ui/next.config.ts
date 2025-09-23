import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: 'standalone',
  async rewrites() {
    // 在构建时使用环境变量，运行时通过Docker环境变量传入
    const managerHost = process.env.MANAGER_HOST || 'sysarmor-manager-1';
    const managerPort = process.env.MANAGER_PORT || '8080';

    return [
      {
        source: '/api/:path*',
        destination: `http://${managerHost}:${managerPort}/api/:path*`,
      },
    ];
  },
  typescript: {
    ignoreBuildErrors: true,
  },
  eslint: {
    ignoreDuringBuilds: true,
  },
};

export default nextConfig;
