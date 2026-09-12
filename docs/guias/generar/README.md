# Cómo se generan las guías

Las dos guías en PDF (`../SGRC-guia-docentes.pdf` y
`../SGRC-guia-administradores.pdf`) no se escriben a mano: el texto vive en los
dos `.html` de la carpeta de arriba y **todas las capturas salen del sistema
corriendo de verdad**. El mismo pipeline actualiza las capturas del README
(`../../capturas/`).

```bash
cp .env.capturas.example .env.capturas   # una sola vez
make capturas
```

Eso es todo. `capturar-todo.sh` levanta su propia pila en el proyecto de
Compose `sgrc-capturas` —base recién creada, datos neutros—, siembra, saca las
capturas en el orden que hace falta, las numera, las recorta, arma los dos PDF
y baja la pila, pase lo que pase. La base de desarrollo no se toca: es otro
proyecto y por lo tanto otro volumen.

Después, `git status` dice qué cambió. Las pantallas que no se tocaron salen
idénticas byte a byte, **salvo las que muestran una fecha o una hora**: esas
cambian en cada corrida porque los datos se siembran con `now()`. Están
listadas en `capturas-con-reloj.txt`, son once, y la diferencia es siempre la
franja del reloj.

**Y no depende de que nadie se acuerde.** El workflow `capturas.yml` lo corre
solo cuando cambia `frontend/src/**` o `docs/guias/**`, y hace dos cosas:

- **Falla si alguna captura no se generó** —`--estricto`—, que es lo que atrapa
  un selector podrido. Vale para las sesenta.
- **Falla si una captura sin reloj dejó de coincidir** con lo que muestra el
  sistema: la interfaz se movió y nadie regeneró. Las once con reloj se informan
  y no frenan nada; compararlas por igualdad las pondría en rojo todas las
  semanas, y una alarma que suena siempre deja de mirarse.

Ese es el punto: hasta la 1.21 el pipeline estaba automatizado pero había que
invocarlo, y por eso las capturas envejecían igual.

## Cuando sólo cambia el texto o la versión

```bash
make capturas-pdf
```

Rehace únicamente los dos documentos, sin levantar nada y sin Docker: dos
minutos. Es lo que hace falta cuando sube la versión —la portada es parte del
documento— o cuando se corrige el texto de un capítulo. Las capturas muestran a
propósito el pie de la versión anterior: se sacan antes del bump, y regenerar
cincuenta imágenes por un dígito no se justifica.

## Lo que hace falta tener instalado

- **Docker**, para la pila de capturas.
- **Node**, para Playwright. Si no hay en la máquina, `capturar-todo.sh` usa
  la imagen oficial `mcr.microsoft.com/playwright:v<versión>-noble` —que ya
  trae el navegador— y resuelve sola la versión leyendo
  `frontend/package.json`. No hay nada que configurar.
- **Python con Pillow**, para recortar y numerar.
- **`jq` y `curl`**, para los scripts de datos.
- **`pdftotext`** (opcional): si está, el script verifica que los PDF no
  salieran truncados.

## Correr una sola parte

Cada script de captura es independiente y se puede correr suelto contra una
pila ya levantada, que es lo cómodo cuando se está peleando con UNA pantalla:

```bash
export SALIDA=/tmp/capturas-sgrc
node docs/guias/generar/capturar-formularios.mjs
python3 docs/guias/generar/preparar-imagenes.py "$SALIDA" docs/guias/imagenes
```

Sin `--estricto`, `preparar-imagenes.py` saltea con `✗ falta` lo que no
encuentra, que es lo correcto en una corrida parcial. **`capturar-todo.sh` sí lo
pasa**, y ahí faltar es un error que corta: ese mensaje era idéntico cuando se
salteaba un script a propósito y cuando uno fallaba, y así estuvieron rotas
cuatro capturas durante semanas sin que nada lo señalara.

Dos cosas al correr suelto:

- **`capturar-jornada.mjs` deja la jornada declarada, y los demás la
  necesitan.** Sobre una base nueva, cualquier otro script corrido primero deja
  al Admin atrapado en el asistente de la primera jornada, y todas sus capturas
  salen mostrando eso.
- **El login falla de a ratos con `waitForURL: Timeout`** cuando se encadenan
  varias corridas. No es la contraseña —el mismo login por curl da 200—:
  repetir el script alcanza. `capturar-todo.sh` ya reintenta una vez por eso.

## `capturar-jornada.mjs` va primero, y no es un detalle de orden

Los dos motivos están escritos arriba del script, pero conviene tenerlos acá:

1. **La primera captura necesita que la escuela NO tenga jornada declarada.**
   Es lo que ve un Admin al entrar a un sistema recién instalado, y cualquier
   script que corra antes y declare una la deja imposible de tomar.
2. **Al terminar, deja la jornada declarada, y los demás la necesitan.** Sin
   ningún tramo cargado el sistema le pide a cada Admin que la declare y no lo
   deja navegar: todas las capturas de Admin que vengan después mostrarían el
   asistente en vez de su pantalla.

La confirmación del impacto se toma **sin aplicarla**: se propone un horario
que deja clases afuera, se fotografía la pregunta y se sale con «Volver sin
cambiar nada», así los datos de demostración quedan intactos.

## Decisiones que conviene no deshacer

- **Las credenciales van por entorno.** Este repositorio es público y la
  instalación de cada escuela tiene las suyas. Ningún script las trae adentro.
- **Los datos de demostración son neutros a propósito** —Ana Gómez, Carro 1,
  1°A— y ninguna captura muestra correos ni nombres reales. La pila de capturas
  arranca siempre de una base vacía, así que no hay forma de que se cuele un
  dato real de la instalación: no es una precaución que haya que recordar.
- **El Admin y el docente salen de `.env.capturas`, y terminan en los PDF.**
  `SEED_ADMIN_EMAIL` es la cuenta que se ve en Usuarios, en Mi perfil y en el
  pie de los avisos; `DOCENTE_EMAIL` es la que siembra el overlay de
  desarrollo. Con el `.env` de desarrollo copiado sin mirar, las capturas
  publican el correo personal en un repositorio público — pasó dos veces. Por
  eso hay un `.env.capturas` aparte, por eso `capturar-todo.sh` **se niega a
  arrancar** si detecta un dominio personal, y por eso el ejemplo trae
  `SMTP_HOST` vacío: el pipeline aprueba cuentas y cancela reservas, y con un
  servidor configurado esos correos se mandan de verdad.
- **Las capturas son imágenes, así que buscar texto en el PDF no alcanza para
  auditarlas.** Un `pdftotext` no ve lo que dice una pantalla fotografiada: si
  hace falta confirmar que no quedó nada personal, hay que mirar las imágenes
  o revisar la base con `make psql` antes de capturar.
- **`preparar-imagenes.py` corta el vacío de abajo salteando el pie de la
  página web.** Sin eso, el hueco entre lo último que importa y el
  «SGRC v1.12.0 — software libre…» viaja al PDF como media hoja en blanco.
- **Las capturas numeradas se arman solas.** `capturar-marcas.mjs` guarda las
  coordenadas de cada elemento en `marcas.json` y `marcar.py` dibuja los globos
  rojos encima. Si movés un rótulo en la interfaz, el número lo sigue: no hay
  ninguna coordenada escrita a mano.
- **El texto de las guías está en los `.html`.** El PDF se arma con el Chromium
  de Playwright, que es lo que permite tener numeración de páginas al pie.
