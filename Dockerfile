# One image for both modes: `serve` for the web interface and `nightly` for
# the scheduled crawl. It carries Chromium for pages that need JavaScript.
# Build for linux/amd64, the only platform Scaleway Serverless Jobs runs.
#
# The stage `dev` is the toolbox for working on a laptop without installing
# anything but Docker: Go, Node.js and Chromium. compose.yaml runs make in it.
# A plain docker build skips it.

FROM golang:1.27-bookworm AS dev
COPY --from=node:22-bookworm-slim /usr/local/bin/node /usr/local/bin/node
COPY --from=node:22-bookworm-slim /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
 && ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx \
 && apt-get update \
 && apt-get install -y --no-install-recommends chromium \
 && rm -rf /var/lib/apt/lists/*
ENV CHROME_NO_SANDBOX=1 \
    SPEAKERTRAIL_IN_DOCKER=1
WORKDIR /src

FROM node:22-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/speakertrail ./cmd/speakertrail

# chromedp's headless-shell is Chromium without the desktop parts.
FROM chromedp/headless-shell:stable
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --uid 10001 --create-home app
COPY --from=build /out/speakertrail /app/speakertrail
ENV CHROME_PATH=/headless-shell/headless-shell \
    CHROME_NO_SANDBOX=1 \
    PORT=8080
USER app
EXPOSE 8080
ENTRYPOINT ["/app/speakertrail"]
CMD ["serve"]
