// Estos tests no necesitan Docker ni base de datos: leen los .sql embebidos.
// Cuidan la convención de la que depende que una actualización no se lleve
// los datos puestos, y que ninguna herramienta comprueba sola.
//
// El test que sí levanta un Postgres y aplica una migración sobre una base
// con datos adentro es `migraciones_incrementales_test.go`, detrás del build
// tag `integration`.
package migrations_test

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/ramiro/sgrc/migrations"
)

// nombreDeMigracion es la convención: tres dígitos, guión bajo, un nombre en
// minúsculas. Los tres dígitos importan porque goose ordena por el número,
// pero las herramientas de al lado (el `ls`, el diff de git, esta suite)
// ordenan por texto: sin el relleno, la 10 se leería antes que la 2.
var nombreDeMigracion = regexp.MustCompile(`^(\d{3})_[a-z0-9_]+\.sql$`)

// huellaDelPuntoDePartida es el SHA-256 del SQL de 001_esquema_inicial.sql
// —solo el SQL: las líneas de comentario y las vacías se descartan, así que
// mejorar la explicación de una tabla no rompe nada—.
//
// Existe porque el error que este proyecto no puede permitirse no da síntomas
// en desarrollo: agregar una columna editando la 001 funciona perfecto contra
// una base que nace de cero, y no llega nunca a una base que ya la tiene
// aplicada, porque goose no vuelve a leer un archivo que ya corrió. El
// sistema arranca sano y falla más tarde, en producción, con un 500 en la
// primera pantalla que toca la columna que no está.
const huellaDelPuntoDePartida = "bd6dc1c70e32b53a5ac14e91c8e02eda44547e4b72449a4a65a4dd5e63eda111"

func archivos(t *testing.T) []string {
	t.Helper()
	entradas, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("no se pudo leer el directorio embebido: %v", err)
	}
	var nombres []string
	for _, e := range entradas {
		nombres = append(nombres, e.Name())
	}
	sort.Strings(nombres)
	return nombres
}

func TestLasVersionesSonCorrelativasYNoSeRepiten(t *testing.T) {
	vistas := map[int]string{}
	var versiones []int

	for _, nombre := range archivos(t) {
		coincidencia := nombreDeMigracion.FindStringSubmatch(nombre)
		if coincidencia == nil {
			t.Errorf("%q no sigue la convención NNN_nombre_en_minusculas.sql", nombre)
			continue
		}
		version, err := strconv.Atoi(coincidencia[1])
		if err != nil {
			t.Fatalf("%q: %v", nombre, err)
		}
		if anterior, repetida := vistas[version]; repetida {
			// Dos personas numerando 002 en ramas distintas es el choque
			// clásico. goose aplicaría una sola y la otra quedaría muda.
			t.Errorf("la versión %03d está dos veces: %q y %q", version, anterior, nombre)
		}
		vistas[version] = nombre
		versiones = append(versiones, version)
	}

	sort.Ints(versiones)
	for i, version := range versiones {
		if esperada := i + 1; version != esperada {
			t.Errorf("falta la versión %03d: después de la %03d viene la %03d", esperada, esperada-1, version)
			break
		}
	}
}

func TestCadaMigracionDiceComoSubirYComoBajar(t *testing.T) {
	// goose parte el archivo por estas dos marcas. Sin la de subida no aplica
	// nada; sin la de bajada, el día que haga falta revertir no hay a dónde
	// volver. Se arman por concatenación a propósito: goose lee TODAS las
	// líneas que llevan su marca, y este archivo no es un .sql, pero la
	// costumbre de no escribirlas enteras evita el accidente al copiar.
	subida := "-- +goose " + "Up"
	bajada := "-- +goose " + "Down"

	for _, nombre := range archivos(t) {
		contenido, err := migrations.FS.ReadFile(nombre)
		if err != nil {
			t.Fatalf("no se pudo leer %q: %v", nombre, err)
		}
		texto := string(contenido)
		for marca, cuantas := range map[string]int{
			subida: strings.Count(texto, subida),
			bajada: strings.Count(texto, bajada),
		} {
			if cuantas != 1 {
				t.Errorf("%q tiene %d veces %q; tiene que tener exactamente una", nombre, cuantas, marca)
			}
		}
		if strings.Index(texto, subida) > strings.Index(texto, bajada) {
			t.Errorf("%q pone la bajada antes que la subida", nombre)
		}
	}
}

func TestElPuntoDePartidaEstaCongelado(t *testing.T) {
	contenido, err := migrations.FS.ReadFile("001_esquema_inicial.sql")
	if err != nil {
		t.Fatalf("no se pudo leer el esquema inicial: %v", err)
	}

	huella := huellaDelSQL(string(contenido))
	if huella == huellaDelPuntoDePartida {
		return
	}

	t.Errorf(`cambió el SQL de 001_esquema_inicial.sql (huella nueva: %s).

La 001 es el punto de partida y está congelada: una base que ya la aplicó no
vuelve a leerla nunca, así que este cambio llegaría a las instalaciones nuevas
y a ninguna otra. Lo que quisiste hacer va en un archivo nuevo —002, 003…—,
que es lo único que goose sí aplica sobre una base existente.

Actualizar la constante huellaDelPuntoDePartida solo es correcto si NINGUNA
instalación tiene la 001 aplicada todavía, o sea si la próxima puesta en
marcha va igual desde cero. Ver docs/11-operacion.md §5.`, huella)
}

// huellaDelSQL resume lo que el archivo le hace a la base, ignorando lo que
// le cuenta a quien lo lee: fuera las líneas de comentario y las vacías.
func huellaDelSQL(contenido string) string {
	var instrucciones []string
	for _, linea := range strings.Split(contenido, "\n") {
		linea = strings.TrimRight(linea, " \t\r")
		if desnuda := strings.TrimSpace(linea); desnuda == "" || strings.HasPrefix(desnuda, "--") {
			continue
		}
		instrucciones = append(instrucciones, linea)
	}
	suma := sha256.Sum256([]byte(strings.Join(instrucciones, "\n")))
	return hex.EncodeToString(suma[:])
}

// ultimaVersionDeLosArchivos es la versión a la que debería quedar cualquier
// base al día. La usa el test de integración de al lado.
func ultimaVersionDeLosArchivos(t *testing.T) int64 {
	t.Helper()
	var ultima int64
	for _, nombre := range archivos(t) {
		coincidencia := nombreDeMigracion.FindStringSubmatch(nombre)
		if coincidencia == nil {
			continue
		}
		version, err := strconv.ParseInt(coincidencia[1], 10, 64)
		if err != nil {
			t.Fatalf("%q: %v", nombre, err)
		}
		if version > ultima {
			ultima = version
		}
	}
	if ultima == 0 {
		t.Fatal("no se encontró ninguna migración en el directorio embebido")
	}
	return ultima
}
