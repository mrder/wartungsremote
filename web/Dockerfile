# Build the admin dashboard as static files. Serving them (plus proxying
# /api to wr-core) is handled by the caddy config this image ships with —
# see deployment/docker/Caddyfile.web.
FROM node:22-alpine AS build
WORKDIR /src

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

FROM caddy:2-alpine AS runtime
COPY --from=build /src/dist /srv
