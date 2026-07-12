FROM node:24-alpine AS build
WORKDIR /app
RUN corepack enable
COPY web/manager/package.json web/manager/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/manager/ ./
ENV AUTH_SECRET_FILE=/tmp/build-secret \
    SYSARMOR_BOOTSTRAP_ADMIN_USERNAME_FILE=/tmp/build-username \
    SYSARMOR_BOOTSTRAP_ADMIN_PASSWORD_FILE=/tmp/build-password
RUN umask 077 && printf '%s\n' build-only-not-a-runtime-secret >/tmp/build-secret \
    && printf '%s\n' build-admin >/tmp/build-username \
    && printf '%s\n' build-password >/tmp/build-password
RUN pnpm build

FROM node:24-alpine
ENV HOSTNAME=0.0.0.0 PORT=4173 NODE_ENV=production
WORKDIR /app
COPY --from=build --chown=node:node /app/.next/standalone ./
COPY --from=build --chown=node:node /app/.next/static ./.next/static
USER node
EXPOSE 4173
CMD ["node", "server.js"]
