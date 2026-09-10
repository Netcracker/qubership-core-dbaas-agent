FROM --platform=$BUILDPLATFORM golang:1.27.1@sha256:f44f6e88636cfb311f9ebace870ded69d943f227bb3cb27d32ffd84ea18c43ea AS build

WORKDIR /app

COPY dbaas-agent-service/ .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -o dbaas-agent-service .

FROM ghcr.io/netcracker/qubership-core-base:2.4.3@sha256:346237ade5d4a91a2ba3099aa9377e744b40241b105bc580fb48ad81f56137a1 AS run

COPY --chown=10001:0 --chmod=555 --from=build app/dbaas-agent-service /app/dbaas-agent
COPY --chown=10001:0 --chmod=444 --from=build app/application.yaml /app/
COPY --chown=10001:0 --chmod=444 --from=build app/docs/swagger.json /app/
COPY --chown=10001:0 --chmod=444 --from=build app/docs/swagger.yaml /app/

WORKDIR /app

CMD ["/app/dbaas-agent"]