FROM golang:1.25-alpine AS backend
WORKDIR /src
RUN apk add --no-cache build-base
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/private-agent .

FROM node:22-alpine AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H -u 10001 agent
WORKDIR /app
COPY --from=backend /out/private-agent /app/private-agent
COPY --from=frontend /src/web/dist /app/web/dist
RUN mkdir -p /data/audit && chown -R agent:agent /data /app
USER agent
EXPOSE 18080
ENTRYPOINT ["/app/private-agent"]

