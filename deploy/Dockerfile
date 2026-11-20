# Multi-stage build for all three Go services and the static dashboard.

FROM golang:1.27-alpine AS gobuild
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/edge ./cmd/edge

FROM node:20-alpine AS dashboard
WORKDIR /app
COPY dashboard/package.json dashboard/package-lock.json ./
RUN npm ci
COPY dashboard ./
ENV NEXT_PUBLIC_API_URL=""
RUN npm run build

FROM alpine:3.22 AS api
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
COPY --from=gobuild /out/api /usr/local/bin/api
COPY --from=dashboard /app/out /srv/dashboard
USER app
EXPOSE 4000
ENTRYPOINT ["api"]

FROM alpine:3.22 AS worker
# git is needed to clone sources on the worker (never inside the sandbox).
RUN apk add --no-cache ca-certificates git
COPY --from=gobuild /out/worker /usr/local/bin/worker
ENTRYPOINT ["worker"]

FROM alpine:3.22 AS edge
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
COPY --from=gobuild /out/edge /usr/local/bin/edge
USER app
EXPOSE 8080
ENTRYPOINT ["edge"]
