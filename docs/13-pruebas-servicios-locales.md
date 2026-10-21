# Pruebas locales de Google y SMTP

Estas pruebas usan los adaptadores del backend y se activan por separado.
La suite normal y la integración habitual no envían correos ni usan tokens
Google reales. No requieren cambios en el frontend.

Los comandos siguientes se ejecutan desde `sgrc-monolotico`. Usan Docker y
Go 1.26.10; los volúmenes conservan la caché de módulos y compilación.
La configuración queda en `.env`, que está excluido de Git. No hace falta
copiar contraseñas ni tokens en registros de ejecución o documentación.

## SMTP

Completá el bloque de `.env`:

```dotenv
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=tu-cuenta@gmail.com
SMTP_PASSWORD=tu-contraseña-de-aplicacion
SMTP_FROM=tu-cuenta@gmail.com
SMTP_FROM_NAME=SGRC local
```

Para Gmail, usá una contraseña de aplicación con la verificación en dos
pasos habilitada. Algunas cuentas tienen restricciones que impiden crearla;
consultá la [ayuda oficial de Google](https://support.google.com/accounts/answer/185833?hl=es).
Para otro proveedor, usá su host, usuario y credenciales de SMTP con STARTTLS.
El backend exige STARTTLS y un certificado válido. El puerto 465 con TLS
implícito no está soportado por este adaptador.

Reemplazá `TU_CASILLA_PROPIA` por una casilla que controles. El comando
**envía un correo real** a esa única dirección:

```bash
docker run --rm --env-file .env \
  -e SGRC_TEST_SMTP_REAL=1 \
  -e SGRC_TEST_SMTP_TO=TU_CASILLA_PROPIA \
  -v "$PWD:/src:ro" -w /src \
  -v sgrc-h01-gomod:/go/pkg/mod \
  -v sgrc-h01-gobuild:/root/.cache/go-build \
  golang:1.26.10-bookworm \
  go test -tags integration ./internal/shared/email \
  -run '^TestSMTPReal_EnvioAutorizado$' -count=1 -v
```

La prueba falla si `SMTP_HOST` está vacío, la configuración está incompleta,
falla TLS/autenticación o el servidor rechaza el mensaje. `PASS` significa
que SMTP lo aceptó: confirmá también que apareció en la casilla o en spam.
Este envío prueba el adaptador; no recorre la recuperación de contraseña ni
los eventos de notificaciones. Esos flujos tienen sus pruebas del backend.

## Google

El backend verifica ID tokens de Google. No usa un client secret ni inicia
la interacción de acceso con Google: recibe `credential` en
`POST /api/auth/google` o `POST /api/auth/google/registro`.

1. Usá un cliente OAuth de tipo **aplicación web** y configurá su ID en
   `GOOGLE_CLIENT_ID` del `.env`. Si restringís cuentas por dominio, completá
   `GOOGLE_DOMINIOS_PERMITIDOS`; vacío permite cualquier dominio.
2. Para el acceso interactivo local, registrá `http://localhost` y el origen
   exacto con el puerto usado para iniciar el login. Google explica estos
   valores en su [guía de configuración](https://developers.google.com/identity/gsi/web/guides/get-google-api-clientid).
   No es necesario modificar orígenes para verificar un ID token ya emitido.
3. Conseguí un **ID token reciente emitido para ese mismo client ID** desde
   una sesión Google autorizada. El valor se llama `credential` en Google
   Identity Services. Un access token, un JWT del SGRC o un token emitido para
   otro client ID no sirven.
4. Guardá sólo el ID token en `../sgrc-datos/id-token-google.txt`, fuera del
   repositorio, con permisos `0600`. La carpeta debe tener permisos `0700`.
   No lo incluyas en argumentos de comandos, logs ni archivos versionados.

```bash
docker run --rm --env-file .env \
  -e SGRC_TEST_GOOGLE_REAL=1 \
  -e SGRC_TEST_GOOGLE_TOKEN_FILE=/id-token-google.txt \
  -v "$PWD/../sgrc-datos/id-token-google.txt:/id-token-google.txt:ro" \
  -v "$PWD:/src:ro" -w /src \
  -v sgrc-h01-gomod:/go/pkg/mod \
  -v sgrc-h01-gobuild:/root/.cache/go-build \
  golang:1.26.10-bookworm \
  go test -tags integration ./internal/auth/infrastructure \
  -run '^TestGoogleReal_IDToken$' -count=1 -v
```

La prueba consulta las claves públicas reales y verifica firma, audiencia,
emisor, expiración, dominio e identidad con email verificado. No imprime el
token ni la identidad, ni crea o modifica cuentas del SGRC. Si el token
vence, obtené uno nuevo.

Para comprobar además el ingreso HTTP de una cuenta Google ya registrada y
aprobada, con el backend local accesible en `http://localhost:8080`, ejecutá:

```bash
python3 - <<'PY'
import json
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request, urlopen

token = Path('../sgrc-datos/id-token-google.txt').read_text().strip()
request = Request(
    'http://localhost:8080/api/auth/google',
    data=json.dumps({'credential': token}).encode(),
    headers={'Content-Type': 'application/json'},
    method='POST',
)
try:
    with urlopen(request, timeout=20) as response:
        data = json.load(response)
        print('HTTP', response.status, '- sesión emitida:', bool(data.get('token')))
except HTTPError as error:
    print('HTTP', error.code, '- revisar registro, aprobación y configuración de Google')
PY
```

Este último paso usa la base a la que esté conectado tu backend y puede
actualizar el vínculo Google de la cuenta. Hacelo con una cuenta y base de
prueba. La verificación del adaptador anterior puede pasar y el ingreso HTTP
fallar por las reglas de registro, aprobación o baja: son comprobaciones
distintas. Retirá el archivo del ID token cuando termines.

Para que pueda ejecutar las verificaciones desde este workspace más
adelante, bastará indicar que `.env` está completo, la ruta del archivo del
ID token y autorizar una casilla propia para el envío de prueba.
