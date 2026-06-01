FROM --platform=$BUILDPLATFORM golang:1.26@sha256:2d6c80227255c3112a4d08e67ba98e58efd3846daf15d9d7d4c389565d881b1a AS build

WORKDIR /app

COPY dbaas-agent-service/ .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -o dbaas-agent-service .

FROM ghcr.io/netcracker/qubership-core-base:2.2.13@sha256:39304438e598101578f91d84bc58699fd1a3f717a46b786cb43523735826e7b4 AS run

COPY --chown=10001:0 --chmod=555 --from=build app/dbaas-agent-service /app/dbaas-agent
COPY --chown=10001:0 --chmod=444 --from=build app/application.yaml /app/
COPY --chown=10001:0 --chmod=444 --from=build app/docs/swagger.json /app/
COPY --chown=10001:0 --chmod=444 --from=build app/docs/swagger.yaml /app/

WORKDIR /app

USER 10001:10001

CMD ["/app/dbaas-agent"]