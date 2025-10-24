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
    // 支持宿主机开发和容器部署两种模式
    const isHostDevelopment = process.env.NODE_ENV === 'development' && !process.env.UI_MANAGER_HOST;

    let managerHost: string;
    let managerPort: string;

    if (isHostDevelopment) {
      // 宿主机开发模式：直接连接 localhost
      managerHost = process.env.NEXT_PUBLIC_API_HOST || 'localhost';
      managerPort = process.env.NEXT_PUBLIC_API_PORT || '8080';
    } else {
      // Docker 容器模式：使用内部网络地址
      managerHost = process.env.UI_MANAGER_HOST || 'sysarmor-manager-1';
      managerPort = process.env.UI_MANAGER_PORT || '8080';
    }

    console.log(`🔗 API Proxy: /api/* -> http://${managerHost}:${managerPort}/api/*`);
    console.log(`📍 Mode: ${isHostDevelopment ? 'Host Development' : 'Docker Container'}`);

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
