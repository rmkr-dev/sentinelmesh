FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/platform ./cmd/platform \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/obsctl ./cmd/obsctl \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/demo ./cmd/demo \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/trafficgen ./cmd/trafficgen

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/platform /out/obsctl /out/demo /out/trafficgen /usr/local/bin/
COPY config /app/config
COPY web /app/web
COPY prompts /app/prompts
COPY runbooks /app/runbooks
USER nonroot:nonroot
ENV PLATFORM_CONFIG=/app/config \
    WEB_DIR=/app/web \
    RUNBOOK_DIR=/app/runbooks \
    PROMPT_DIR=/app/prompts
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/platform"]
