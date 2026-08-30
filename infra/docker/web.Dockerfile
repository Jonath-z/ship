ARG NODE_IMAGE=node:22.23.2-alpine3.23

FROM ${NODE_IMAGE} AS builder
RUN corepack enable && corepack prepare pnpm@9.12.0 --activate
WORKDIR /workspace
COPY . .
RUN pnpm install --frozen-lockfile && pnpm --filter @ship/web build

FROM ${NODE_IMAGE} AS runtime
ARG BUILD_SHA=development
ARG VERSION=dev
ARG SOURCE_URL=https://github.com/Jonath-z/ship
LABEL org.opencontainers.image.title="Ship Web" \
      org.opencontainers.image.description="Ship control-plane web application" \
      org.opencontainers.image.source="${SOURCE_URL}" \
      org.opencontainers.image.revision="${BUILD_SHA}" \
      org.opencontainers.image.version="${VERSION}"
ENV NODE_ENV=production
ENV HOSTNAME=0.0.0.0
ENV PORT=3000
RUN addgroup -S ship && adduser -S ship -G ship
WORKDIR /app
COPY --from=builder --chown=ship:ship /workspace/apps/web/.next/standalone ./
COPY --from=builder --chown=ship:ship /workspace/apps/web/.next/static ./apps/web/.next/static
USER ship
EXPOSE 3000
HEALTHCHECK --interval=10s --timeout=6s --start-period=20s --retries=5 CMD ["node", "-e", "fetch('http://127.0.0.1:3000/').then(r=>{if(!r.ok)process.exit(1)}).catch(()=>process.exit(1))"]
CMD ["node", "apps/web/server.js"]
