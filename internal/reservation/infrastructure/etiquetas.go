package infrastructure

import "fmt"

// etiquetaConCarroSQL arma el fragmento que nombra un equipo DENTRO DE UNA
// FRASE: "PC 7 del Carro 2", "Proyector Epson".
//
// Existe porque el nombre de un equipo se resuelve en nueve consultas de este
// paquete y no todas quieren lo mismo. La distinción no es de estilo:
//
//   - Las que devuelven `carro_nombre` como COLUMNA APARTE (la lista de
//     préstamos, los equipos disponibles, los ocupados) NO deben usar esto: la
//     pantalla ya compone "PC 7 · Carro 2" con las dos columnas, y meter el
//     carro adentro de la etiqueta lo duplicaría.
//
//   - Las que producen texto para leer —los avisos al buzón y al correo— sí,
//     porque ahí no hay otra columna donde poner el carro. Y hace falta: el
//     identificador es el número del zócalo, así que "PC 7" existe una vez por
//     carro, y un aviso que dice "todavía no retiraste PC 1, PC 5 y PC 11" no
//     le permite a nadie saber a qué carro ir.
//
// Los alias van por parámetro porque cada consulta nombra sus tablas distinto.
// El LEFT JOIN al carro lo pone quien la usa: un equipo suelto no tiene, y
// para ese el primer COALESCE ya resolvió con su nombre.
func etiquetaConCarroSQL(equipo, carro string) string {
	return fmt.Sprintf(
		"COALESCE(%[1]s.nombre, 'PC ' || %[1]s.identificador || COALESCE(' del ' || %[2]s.nombre, ''))",
		equipo, carro)
}
