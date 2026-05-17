FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json* bun.lock* ./
RUN npm install --legacy-peer-deps
COPY . .

# Vite reads VITE_* from process.env at build time and inlines the
# values into the JS bundle. Docker doesn't pass host env into RUN
# instructions unless explicitly wired, so without these ARGs the
# build would always use empty values regardless of what's set on
# the host. Empty defaults keep a fresh checkout buildable; missing
# values just disable the corresponding feature (no Sentry, no
# Brandfetch logos, etc.).
ARG VITE_BRANDFETCH_CLIENT_ID=""
ARG VITE_SENTRY_DSN=""
ARG VITE_APP_VERSION=""
ARG VITE_BOT_USERNAME="SubGuardBot"
ARG VITE_SUPPORT_URL=""
ARG VITE_PRIVACY_POLICY_URL=""
ARG VITE_TERMS_OF_SERVICE_URL=""
ENV VITE_BRANDFETCH_CLIENT_ID=$VITE_BRANDFETCH_CLIENT_ID
ENV VITE_SENTRY_DSN=$VITE_SENTRY_DSN
ENV VITE_APP_VERSION=$VITE_APP_VERSION
ENV VITE_BOT_USERNAME=$VITE_BOT_USERNAME
ENV VITE_SUPPORT_URL=$VITE_SUPPORT_URL
ENV VITE_PRIVACY_POLICY_URL=$VITE_PRIVACY_POLICY_URL
ENV VITE_TERMS_OF_SERVICE_URL=$VITE_TERMS_OF_SERVICE_URL

RUN npm run build

# The dist/ folder is the output — copied to a shared volume by docker-compose
FROM alpine:3.21
COPY --from=build /app/dist /app/dist
CMD ["echo", "Frontend build complete"]
