# Go 1.25+ is required since the test-infrastructure dependencies
# (testcontainers-go) bumped go.mod's minimum to 1.25.0. Prod build
# itself doesn't ship the test deps (only files under //go:build
# integration use them), but `go mod download` reads the directive and
# refuses to run on a lower toolchain.
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget
COPY --from=build /server /server
EXPOSE 3000
CMD ["/server"]
