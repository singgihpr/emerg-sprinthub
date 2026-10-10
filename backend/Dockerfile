FROM golang:1.24-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/backend .

FROM alpine:3.20

WORKDIR /app

RUN adduser -D -u 10001 appuser
COPY --from=build /out/backend /usr/local/bin/backend

USER appuser

EXPOSE 8000

ENTRYPOINT ["backend"]
