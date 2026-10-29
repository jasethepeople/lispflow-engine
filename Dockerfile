# ---- build stage ----
FROM golang:1.22-alpine AS build
WORKDIR /src

COPY go.mod ./
RUN go mod tidy
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/engine-server ./cmd/engine-server

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/engine-server /engine-server
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/engine-server"]
