# Build estático — sin CGO para poder correr sobre scratch (ver docs/06-arquitectura.md §7)
FROM golang:1.25-alpine AS build
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod go.sum* ./
# Las cachés de Go viven en un `type=cache` de BuildKit y no adentro de la
# capa. Eso cambia qué pasa cuando el build falla a mitad de camino.
#
# Sin esto, una descarga interrumpida —un DNS que se cae bajo la ráfaga de
# consultas, una red que corta— hace que BuildKit descarte la capa entera y
# el reintento vuelva a bajar los más de doscientos módulos desde cero. En
# una conexión inestable eso no converge nunca: alcanza con que UNA falle
# para perder las otras doscientas. Con la caché, lo que se bajó sobrevive al
# fallo y cada reintento arranca donde quedó el anterior.
#
# El mismo mount va en los dos pasos, y no es una repetición por prolijidad:
# como el directorio no queda en la capa, sin él `go build` no encontraría
# nada de lo que `go mod download` acaba de traer y lo bajaría de nuevo.
#
# La caché es del constructor, no de la imagen: no engorda el binario final
# —que igual sale de `scratch`— pero sí ocupa disco en el servidor. Se limpia
# con `docker builder prune` si algún día molesta.
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
# GOCACHE además del módulo: acá la que ahorra es la caché de compilación, que
# hace que un cambio en un solo paquete no recompile todas las dependencias.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /out/sgrc-app ./cmd

FROM scratch
# Los certificados raíz. En scratch no hay ninguno, y Go los lee del disco:
# sin este archivo TODO handshake TLS saliente falla con "x509: certificate
# signed by unknown authority". Rompe las dos únicas cosas que salen a
# internet, y las dos fallan de formas que no se parecen entre sí:
#
#   - El ingreso con Google no puede traer el JWKS con las claves públicas
#     (ver infrastructure/google_idtoken.go). Es una falla de red, no un
#     token inválido, así que el handler la devuelve como 500 "error
#     interno" y desde el navegador parece un bug del login.
#   - El STARTTLS contra smtp.gmail.com no negocia (ver shared/email). El
#     envío corre en una goroutine del bus, donde el error solo se loguea:
#     el alta del usuario se completa y el mail no sale nunca.
#
# La zona horaria tiene el mismo problema (en scratch tampoco hay
# /usr/share/zoneinfo) y está resuelto de otra forma: cmd/main.go importa
# time/tzdata y la embebe en el binario. Para las CAs no conviene el
# equivalente (x509.SystemCertPool con las de Go), porque congelaría el
# almacén de confianza en la fecha del build.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/sgrc-app /sgrc-app

# El proceso corría como root. No hay nada acá que necesite privilegios —
# escucha en 8080 (no en un puerto privilegiado), no escribe archivos y no
# lee nada del sistema— así que ser uid 0 solo agregaba superficie: si
# alguna vez se monta un volumen o se encadena con un escape del runtime,
# la diferencia entre root y un uid cualquiera es la diferencia entre
# comprometer el host y no.
#
# El uid va numérico y no por nombre porque la imagen es `scratch`: no hay
# /etc/passwd donde resolver un usuario. 65532 es el "nonroot" convencional
# de las imágenes distroless. El binario queda 0755 desde la etapa de build,
# así que cualquier uid puede ejecutarlo.
USER 65532:65532
EXPOSE 8080
# El chequeo lo hace el propio binario contra su /health (ver
# cmd/healthcheck.go): en una imagen scratch no hay shell ni curl con los
# que armarlo desde afuera. start-period cubre el arranque, que incluye
# conectar a Postgres y sembrar el Admin inicial.
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD ["/sgrc-app", "healthcheck"]
ENTRYPOINT ["/sgrc-app"]
