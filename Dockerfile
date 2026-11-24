# Imagen del MID. Es el unico servicio que se publica a internet: las 7 APIs
# CRUD quedan en la red interna y el MID las llama por MID_URL_*.
#
#   docker build -t jobsy-mid .
#
# Los archivos que suben los usuarios se guardan en /app/archivos: montar ahi
# un volumen, o se pierden en cada actualizacion del contenedor.

FROM golang:1.27-alpine AS compilacion
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/mid .

FROM alpine:3.22
# ca-certificates: el MID llama por HTTPS a las pasarelas de pago.
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 jobsy \
    && mkdir -p /app/archivos && chown jobsy /app/archivos
WORKDIR /app
COPY --from=compilacion /out/mid ./mid
COPY --from=compilacion /src/conf ./conf
USER jobsy
VOLUME ["/app/archivos"]
EXPOSE 9090
ENTRYPOINT ["./mid"]
