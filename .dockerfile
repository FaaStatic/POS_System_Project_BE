FROM golang:1.27.2-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -tags timetzdata -trimpath -ldflags="-s -w" -o /out/pos main.go

FROM alpine:3.22
RUN adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/pos /app/pos
COPY swagger.json ./
USER app
EXPOSE 3030
ENTRYPOINT ["/app/pos"]
CMD ["serve"]
