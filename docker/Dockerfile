FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG CMD=api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${CMD}

FROM alpine:3.19
RUN apk add --no-cache ffmpeg ca-certificates
COPY --from=build /out/app /app
COPY web/ /web/
ENTRYPOINT ["/app"]
