FROM node:18-alpine AS base

# Install dependencies only when needed
FROM base AS deps
# Check https://github.com/nodejs/docker-node/tree/b4117f9333da4138b03a546ec926ef50a31506c3#nodealpine to understand why libc6-compat might be needed.
RUN apk add --no-cache libc6-compat
WORKDIR /app

# Install dependencies based on the preferred package manager
COPY package.json pnpm-lock.yaml* ./
# Configure Chinese mirrors for better network connectivity
RUN npm config set registry https://registry.npmmirror.com/ && \
    corepack enable pnpm && \
    pnpm config set registry https://registry.npmmirror.com/ && \
    pnpm i --no-frozen-lockfile

# Rebuild the source code only when needed
FROM base AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .

# Build-time environment variables for UI container internal access
ARG UI_MANAGER_HOST=sysarmor-manager-1
ARG UI_MANAGER_PORT=8080
ENV UI_MANAGER_HOST=$UI_MANAGER_HOST
ENV UI_MANAGER_PORT=$UI_MANAGER_PORT

# Build-time environment variables for external APIs
ARG THREAT_API_BASE_URL=
ARG THREAT_API_ENABLED=false
ENV NEXT_PUBLIC_THREAT_API_BASE_URL=$THREAT_API_BASE_URL
ENV NEXT_PUBLIC_THREAT_API_ENABLED=$THREAT_API_ENABLED

# Next.js collects completely anonymous telemetry data about general usage.
# Learn more here: https://nextjs.org/telemetry
# Uncomment the following line in case you want to disable telemetry during the build.
# ENV NEXT_TELEMETRY_DISABLED 1

# Configure Chinese mirrors for better network connectivity
RUN npm config set registry https://registry.npmmirror.com/ && \
    corepack enable pnpm && \
    pnpm config set registry https://registry.npmmirror.com/ && \
    pnpm run build

# Production image, copy all the files and run next
FROM base AS runner
WORKDIR /app

ENV NODE_ENV production
# Uncomment the following line in case you want to disable telemetry during runtime.
# ENV NEXT_TELEMETRY_DISABLED 1

RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 nextjs

COPY --from=builder /app/public ./public

# Set the correct permission for prerender cache
RUN mkdir .next
RUN chown nextjs:nodejs .next

# Automatically leverage output traces to reduce image size
# https://nextjs.org/docs/advanced-features/output-file-tracing
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

USER nextjs

EXPOSE 3000

ENV PORT 3000
# set hostname to localhost
ENV HOSTNAME "0.0.0.0"

# server.js is created by next build from the standalone output
# https://nextjs.org/docs/pages/api-reference/next-config-js/output
CMD ["node", "server.js"]
