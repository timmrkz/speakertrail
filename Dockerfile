# One image for both modes: `serve` for the web interface and `nightly` for
# the scheduled crawl. It carries Chromium for pages that need JavaScript.
# Build for linux/amd64, the only platform Scaleway Serverless Jobs runs.

FROM node:22-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/speakertrail ./cmd/speakertrail

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
