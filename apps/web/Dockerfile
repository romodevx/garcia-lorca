# García Lorca — web (Next.js standalone) image for Cloud Run.
#
# Build context is the REPO ROOT (same convention as the Go services), because
# apps/web depends on four `workspace:*` packages and on the pnpm lockfile:
#   docker build -f apps/web/Dockerfile -t web .
#
# The root .dockerignore excludes `apps` and `packages` for the Go images, so
# this Dockerfile ships its own apps/web/Dockerfile.dockerignore, which BuildKit
# prefers. Building without BuildKit (DOCKER_BUILDKIT=0) would pick up the root
# ignore file and fail on the first COPY.

# ---- deps: install only what @garcia-lorca/web needs ----------------------
FROM node:22-bookworm-slim AS deps
WORKDIR /app

RUN corepack enable pnpm

# Manifests first, so a source-only change does not re-resolve dependencies.
# Every workspace member's package.json must be present or the frozen lockfile
# cannot be satisfied — apps/mobile included, even though its sources are not
# copied (see Dockerfile.dockerignore).
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc* ./
COPY apps/web/package.json ./apps/web/
COPY apps/mobile/package.json ./apps/mobile/
COPY packages/api-client/package.json ./packages/api-client/
COPY packages/config/package.json ./packages/config/
COPY packages/domain/package.json ./packages/domain/
COPY packages/i18n/package.json ./packages/i18n/
COPY packages/ui/package.json ./packages/ui/

# `--filter @garcia-lorca/web...` pulls web plus its workspace dependencies and
# skips the Expo toolchain, which this image never runs.
RUN pnpm install --frozen-lockfile --filter @garcia-lorca/web...

# ---- builder --------------------------------------------------------------
FROM node:22-bookworm-slim AS builder
WORKDIR /app

RUN corepack enable pnpm

# Copy the WHOLE installed tree, not just the root and apps/web.
#
# pnpm gives every workspace package its own node_modules of symlinks into
# node_modules/.pnpm. Copying only the root and apps/web left packages/ui and
# packages/i18n without theirs, so `next build` failed on THEIR dependencies —
# cmdk, intl-messageformat, @formatjs/icu-messageformat-parser — even though the
# install had fetched them. The symlinks stay valid because the layout under
# /app is preserved verbatim.
COPY --from=deps /app ./
# Sources last, so a code change does not invalidate the install layer.
COPY . .

# Same-origin API base. "/" (not "") on purpose:
#   * apps/web/app/auth/api.ts and app/features.ts both do
#     `baseURL.replace(/\/+$/, "") + path`, so "/" collapses to "" and yields a
#     correct relative URL — the browser talks only to this service's origin,
#     which is what keeps the SameSite=Strict auth cookies working.
#   * "" would be indistinguishable from "unset" and trips the design-demo
#     branch in app/flags-provider.tsx, serving MOCK data that looks real.
# NEXT_PUBLIC_* is inlined at build time: changing this needs a rebuild, not an
# env update on the Cloud Run service.
ARG NEXT_PUBLIC_API_URL="/"
ENV NEXT_PUBLIC_API_URL=${NEXT_PUBLIC_API_URL}

# Identity Platform. ALL EMPTY BY DEFAULT, which is what keeps the provider
# sign-in off in any image built without them: providerLoginEnabled() needs both
# the flag AND an api key, so a half-configured build cannot half-enable it.
#
# These are inlined into the client bundle, and that is fine — a Firebase web
# config is public by design. The boundary is the gateway and the provider's own
# rules, never the config being secret. Nothing here is a credential.
ARG NEXT_PUBLIC_AUTH_PROVIDER=""
ARG NEXT_PUBLIC_FIREBASE_API_KEY=""
ARG NEXT_PUBLIC_FIREBASE_AUTH_DOMAIN=""
ARG NEXT_PUBLIC_FIREBASE_PROJECT_ID=""
ENV NEXT_PUBLIC_AUTH_PROVIDER=${NEXT_PUBLIC_AUTH_PROVIDER}
ENV NEXT_PUBLIC_FIREBASE_API_KEY=${NEXT_PUBLIC_FIREBASE_API_KEY}
ENV NEXT_PUBLIC_FIREBASE_AUTH_DOMAIN=${NEXT_PUBLIC_FIREBASE_AUTH_DOMAIN}
ENV NEXT_PUBLIC_FIREBASE_PROJECT_ID=${NEXT_PUBLIC_FIREBASE_PROJECT_ID}
ENV NEXT_TELEMETRY_DISABLED=1
ENV NODE_ENV=production

RUN pnpm --filter @garcia-lorca/web run build

# Fail the BUILD, not the container, if the standalone server or the workspace
# packages did not make it into the traced output. Cheap insurance against the
# failure mode where the image builds green and dies on boot.
# NOTE the path. `pnpm --filter` runs the build with apps/web as its working
# directory, so the output lands under /app/apps/web/.next — not /app/.next. And
# because outputFileTracingRoot is the monorepo root, Next nests the server one
# more level down, at .next/standalone/apps/web/server.js.
RUN test -f apps/web/.next/standalone/apps/web/server.js \
      || (echo "standalone server.js not found. Either the tracing root moved or this path is stale; run: find apps/web/.next/standalone -name server.js" && exit 1)

# ---- runtime --------------------------------------------------------------
FROM node:22-bookworm-slim AS runner
WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
# Cloud Run injects PORT and routes to it; server.js reads both of these.
ENV PORT=8080
ENV HOSTNAME=0.0.0.0

# The standalone output is NESTED: server.js lands at
# .next/standalone/apps/web/server.js and does process.chdir(__dirname), so the
# static assets and public files must sit next to it under apps/web — not at the
# image root. Getting this wrong renders the page with no CSS or JS and logs
# nothing at all.
COPY --from=builder --chown=node:node /app/apps/web/.next/standalone ./
COPY --from=builder --chown=node:node /app/apps/web/.next/static ./apps/web/.next/static
COPY --from=builder --chown=node:node /app/apps/web/public ./apps/web/public

USER node
EXPOSE 8080

CMD ["node", "apps/web/server.js"]
