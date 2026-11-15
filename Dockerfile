# 1) build the client
FROM node:22-alpine AS client
WORKDIR /client
COPY client/package.json client/package-lock.json ./
RUN npm ci
COPY client/ ./
RUN npm run build

# 2) build the Go server (pure Go, so no C compiler is needed)
FROM golang:1.25-alpine AS server
ENV GOTOOLCHAIN=auto CGO_ENABLED=0
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN go build -o /out/server .

# 3) small runtime image
FROM alpine:3.20
WORKDIR /app
COPY --from=server /out/server /app/server
COPY --from=client /client/dist /app/client
EXPOSE 10000
CMD ["/app/server", "-static", "/app/client", "-db="]
