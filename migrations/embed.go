// Package migrations lleva el esquema adentro del binario.
//
// El embebido no es una comodidad: la imagen final es `FROM scratch` y no
// tiene sistema de archivos más allá del ejecutable, así que un directorio
// de migraciones "al lado del binario" simplemente no existiría ahí. Con
// go:embed, el esquema viaja compilado y el contenedor puede migrar solo.
//
// Efecto secundario que conviene conocer: el archivo .sql se lee en tiempo
// de compilación. Cambiarlo exige recompilar —`docker compose up --build`,
// no `restart`— para que el binario vea la versión nueva.
package migrations

import "embed"

// FS son los archivos .sql de este directorio, que goose aplica en orden de
// versión (el número con el que empieza cada nombre).
//
//go:embed *.sql
var FS embed.FS
