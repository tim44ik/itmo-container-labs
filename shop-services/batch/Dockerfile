FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY batch/main.go ./main.go
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /out/batch ./main.go

FROM scratch
COPY --from=builder /out/batch /batch
USER 65532:65532
ENTRYPOINT ["/batch"]
