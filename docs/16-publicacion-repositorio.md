# Qué se publica y qué se mantiene privado

El repositorio distribuye el sistema, sus pruebas, documentación y plantillas
para que cualquier institución pueda instalarlo. Cada instalación mantiene
sus propias credenciales, datos y ajustes fuera de los archivos publicados.

| Archivo o contenido | ¿Se publica? | Uso |
|---|---|---|
| `docker-compose.yml` | Sí | Servicios compartidos y despliegue. Lee la configuración privada `.env`. |
| `docker-compose.dev.yml` | Sí | Desarrollo; se aplica explícitamente con `-f`. |
| `docker-compose.local.example.yml` | Sí | Plantilla de una copia local independiente, sin respaldos montados. |
| `.env.example`, `.env.local.example`, `frontend/.env.example` | Sí | Variables documentadas con marcadores o valores ficticios. |
| `docker-compose.local.yml`, `.env`, `.env.local`, `frontend/.env*` activos | No | Ajustes y secretos de cada instalación. |
| `*compose*.override.yml` y variantes `.yaml` | No | Sobrescrituras privadas, incluidas las que Compose carga automáticamente. |
| Código, migraciones SQL y configuración de CI, nginx y observabilidad | Sí | Componentes compartidos necesarios para construir y operar el sistema. |
| Guías de administrador y docente, sus imágenes y PDF | Sí | Las capturas documentales se usan exclusivamente en estas guías, con datos ficticios; se generan con `make capturas`. |
| Volcados, exportaciones reales, accesos y datos de usuarios | No | Información propia de la institución. |
| Configuración personal de herramientas y registros locales | No | Archivos de cada máquina. |
| Informes de trabajo, diagnósticos y evidencias de una instalación | No | Se conservan fuera del repositorio; los procedimientos reproducibles se documentan en los manuales compartidos. |
| `ANALISIS_*.md` y variantes con tilde o minúsculas | No | Informes privados de trabajo; se ignoran también si se copian dentro del repositorio. |

## Contenido del producto

Se distribuyen `cmd/`, `internal/`, `migrations/`, `frontend/`, `observabilidad/`,
los scripts vigentes y la configuración compartida de construcción y CI.
`docs/` contiene requisitos, casos de uso, arquitectura, contrato de API,
seguridad, operación, pruebas y guías de uso. No incluye diarios de corrección,
copias de informes temporales ni información de una instalación particular.

Las capturas documentales viven en `docs/guias/imagenes` y se generan con
datos ficticios. El generador vigente consta de `scripts/generar-guias.mjs`
y de `capturar-actual.mjs`, `datos-actuales.mjs`, `hacer-pdf.mjs` y la entrada
del Makefile dentro de `docs/guias/generar/`. El README enlaza las guías.

Los ejemplos utilizan dominios reservados (`example.org`, `.test`) y valores
de demostración. El módulo Go y los enlaces al código identifican el repositorio
público. La atribución legal se conserva en `LICENSE`; la configuración y la
interfaz del producto no incorporan datos de su instalación original.

Los archivos obsoletos `docker-compose.capturas.yml` y `.env.capturas.example`,
la galería `docs/capturas/` y sus generadores antiguos se retiraron del árbol de
trabajo. Sus eliminaciones deben incluirse en el siguiente commit. Las guías
vigentes y su generador se conservan. No es necesario borrar los entornos
activos ni los Compose privados para publicar: se conservan localmente,
excluidos de Git.

## Crear una instalación

Para probar en local, ejecutar `make local-init` y `make local`, como explica
[Entorno local](15-entorno-local.md). El inicializador genera claves aleatorias
y conserva configuraciones existentes. La plantilla de entorno no contiene
claves utilizables y no se usa directamente como configuración activa.

Para producción, crear una copia privada de `.env.example`, completar los
valores propios y seguir [Operación](11-operacion.md). Nunca copiar secretos
reales de vuelta a un archivo `*.example` ni a la documentación.

Las personalizaciones de una máquina se conservan en los archivos ignorados.
Una mejora reutilizable se incorpora a la plantilla correspondiente, sin
rutas personales, nombres reales, claves ni datos de inventario.

## Datos y respaldos

Guardar la información institucional fuera del repositorio o en las carpetas
ignoradas `datos-locales/`, `sgrc-datos/` y `backups/`. Las reglas también cubren
los nombres de accesos y exportaciones locales, los volcados y los archivos de
respaldo comprimidos habituales si se copian a otra carpeta. No se bloquean
todos los archivos SQL: las migraciones del producto deben publicarse.

Git y Docker tienen exclusiones separadas. Los `.dockerignore` evitan que
configuración privada y datos entren en las capas de construcción; `.gitignore`
evita incorporarlos por accidente al preparar un commit.

## Comprobación antes de publicar

```bash
git status --short
git diff --cached --name-only
git check-ignore -v .env.local docker-compose.local.yml datos-locales/archivo.txt
git ls-files -ci --exclude-standard
```

El último comando debe quedar sin salida: señala archivos ya versionados que
ahora coinciden con reglas de exclusión. `.gitignore` no retira por sí solo un
archivo del índice ni borra información de commits anteriores. Revisar el
contenido de los archivos preparados; las exclusiones por nombre no detectan
un secreto escrito dentro de código o documentación pública. Evitar `git add -f`
sobre configuraciones o datos privados.
