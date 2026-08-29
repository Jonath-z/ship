FROM node:20-alpine AS builder
RUN corepack enable && corepack prepare pnpm@9.12.0 --activate
WORKDIR /workspace
COPY . .
RUN pnpm install --frozen-lockfile && pnpm --filter @ship/web build

FROM node:20-alpine AS runtime
ENV NODE_ENV=production
ENV HOSTNAME=0.0.0.0
ENV PORT=3000
RUN addgroup -S ship && adduser -S ship -G ship
WORKDIR /app
COPY --from=builder --chown=ship:ship /workspace/apps/web/.next/standalone ./
COPY --from=builder --chown=ship:ship /workspace/apps/web/.next/static ./apps/web/.next/static
USER ship
EXPOSE 3000
CMD ["node", "apps/web/server.js"]
