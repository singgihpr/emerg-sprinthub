FROM node:20-bookworm AS build

WORKDIR /app

# CRA inlines env vars at build time. Empty value -> frontend/src/lib/api.js
# resolves to "/api", i.e. same origin, served by the nginx proxy below.
ARG REACT_APP_BACKEND_URL=""
ENV REACT_APP_BACKEND_URL=$REACT_APP_BACKEND_URL \
    NODE_OPTIONS=--max-old-space-size=2048 \
    CI=false

COPY package.json package-lock.json .npmrc ./
RUN npm ci

COPY . .
RUN npm run build


FROM nginx:1.27-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/build /usr/share/nginx/html

EXPOSE 80
