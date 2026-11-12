FROM node:24-alpine AS build
WORKDIR /src
COPY package.json pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile
COPY . ./
ARG PUBLIC_WEB_URL
ENV PUBLIC_WEB_URL=$PUBLIC_WEB_URL
RUN pnpm run build

FROM nginx:1.31.8-alpine@sha256:713fc2ebfd92e8217db451477182ee91944b123d2b1ee9fd5a8c6e91ca6c3a11
COPY --from=build /src/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -T 2 -O /dev/null http://127.0.0.1/discover || exit 1
