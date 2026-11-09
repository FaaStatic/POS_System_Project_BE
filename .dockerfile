ARG GO_VERSION=1.27.3

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go mod tidy -diff && go vet ./...
RUN CGO_ENABLED=0 go build -tags timetzdata,nodynamic -trimpath -ldflags="-s -w" -o /out/pos main.go
RUN mkdir -p /out/storage/reports

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/pos /app/pos
COPY swagger.json ./
COPY --from=build --chown=nonroot:nonroot /out/storage /app/storage
EXPOSE 3030
USER nonroot:nonroot
ENTRYPOINT ["/app/pos"]
CMD ["serve"]
