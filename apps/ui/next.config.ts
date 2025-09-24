import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: 'standalone',
  trailingSlash: false,
  generateBuildId: () => 'build',
  webpack: (config, { isServer }) => {
    // 解决EUI组件在构建时的问题
    if (!isServer) {
      config.resolve.fallback = {
        ...config.resolve.fallback,
        fs: false,
        net: false,
        tls: false,
      };
    }

    // 标记EUI为外部依赖，避免构建时的问题
    config.externals = config.externals || [];

    return config;
  },
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
