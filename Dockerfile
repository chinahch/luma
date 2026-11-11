FROM --platform=$BUILDPLATFORM golang:1.26.10-alpine AS build
ARG TARGETOS=linux
ARG TARGETARCH=amd64
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -mod=readonly -trimpath -buildvcs=false -ldflags='-s -w' -o /luma ./cmd/luma

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=build /luma /usr/local/bin/luma
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/luma"]
CMD ["serve"]
