# syntax=docker/dockerfile:1
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web ./
RUN npm run build

FROM golang:1.26.5-alpine AS go
RUN apk add --no-cache build-base git
WORKDIR /src
COPY go.mod go.sum* ./
RUN GOPROXY=direct go mod download
COPY cmd ./cmd
COPY cmsv1connect ./cmsv1connect
COPY internal ./internal
COPY proto ./proto
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=1 go build -trimpath -ldflags='-s -w' -o /out/uvoo-minicms ./cmd/uvoo-minicms

FROM alpine:3.21
RUN apk add --no-cache ca-certificates sqlite-libs && adduser -D -H -u 10001 uvoo-minicms
WORKDIR /app
COPY --from=go /out/uvoo-minicms /app/uvoo-minicms
COPY --from=web /src/web/dist /app/web/dist
RUN mkdir -p /data/uploads && chown -R uvoo-minicms:uvoo-minicms /data /app
USER uvoo-minicms
ENV CMS_ADDR=:8080 CMS_DATA_DIR=/data CMS_DB=/data/cms.db CMS_UPLOAD_DIR=/data/uploads
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/app/uvoo-minicms"]
