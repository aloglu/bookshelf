# Bookshelf's Docker image: one static program, run as an ordinary user.
#
#   docker build -t bookshelf .
#
# The frontend build stage is added with the frontend (docs/v2/PLAN.md, phase 4).

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /bookshelf ./cmd/bookshelf

FROM alpine:3.22
# Certificates for ISBN lookups and cover downloads; time zones for log times.
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /bookshelf /usr/local/bin/bookshelf
ENV BOOKSHELF_DATA_DIR=/data \
    PORT=8080 \
    PUID=1000 \
    PGID=1000
VOLUME /data
EXPOSE 8080
# Bookshelf starts as root only to give /data to PUID:PGID, then drops to that user.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["bookshelf", "health"]
ENTRYPOINT ["bookshelf"]
