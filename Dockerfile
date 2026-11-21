FROM node:22-bookworm-slim AS frontend
WORKDIR /build
RUN npm install --global pnpm@9.15.9
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY . .
ARG VERSION=dev
ENV VITE_APP_VERSION=${VERSION} VITE_GLOB_API_URL=/api
RUN pnpm run type-check && pnpm run build-only

FROM golang:1.26-bookworm AS backend
WORKDIR /build
COPY service/go.mod service/go.sum ./
RUN go mod download
RUN go install github.com/go-bindata/go-bindata/go-bindata@v3.1.2+incompatible
COPY service/ ./
RUN go-bindata -o=assets/bindata.go -pkg=assets assets/... \
    && CGO_ENABLED=1 go build -mod=readonly -trimpath -o /out/sun-panel \
       -ldflags='-X sun-panel/global.RUNCODE=release -X sun-panel/global.ISDOCKER=true' main.go

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=frontend /build/dist /app/web
COPY --from=backend /out/sun-panel /app/sun-panel
EXPOSE 3002
ENTRYPOINT ["/app/sun-panel"]
