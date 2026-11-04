FROM golang:1.26 AS build
WORKDIR /src
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/service ./cmd/api \
    && mkdir -p /runtime/conf /runtime/logs

FROM alpine:3.24
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build --chown=65532:65532 /runtime/ /app/
COPY --from=build /out/service /app/service
WORKDIR /app
USER 65532:65532
ENV TZ=UTC
EXPOSE 8080 9090
STOPSIGNAL SIGTERM
ENTRYPOINT ["/app/service"]
CMD ["-config", "/app/conf/app.yaml"]
