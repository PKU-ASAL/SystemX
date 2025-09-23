import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: 'standalone',
  async rewrites() {
    // UI容器内部访问Manager使用专门的环境变量
    const managerHost = process.env.UI_MANAGER_HOST || 'sysarmor-manager-1';
    const managerPort = process.env.UI_MANAGER_PORT || '8080';

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
