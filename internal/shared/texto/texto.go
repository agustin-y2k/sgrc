// Package texto canoniza y compara los textos que una persona escribe en un
// formulario y que el sistema usa después para decidir si dos cosas son la
// misma: el nombre de un carro, de un equipo, de una materia, de una licencia.
//
// Vive en shared/ y no en el domain de cada módulo porque la regla es una sola
// y no pertenece a ninguno. Antes de existir este paquete, la misma función
// estaba escrita —idéntica, línea por línea— en `academic/domain` y en
// `inventory/domain`, cada una inventada por su lado; y en los otros dos
// lugares que la necesitaban no estaba, que es de donde salieron los
// duplicados que nadie podía ver.
//
// El contrato con la base: Clave() tiene que dar EXACTAMENTE lo mismo que la
// función `clave_texto()` de la migración 011, que es la que sostiene los
// índices únicos. Si se separan, el síntoma es una carga que revienta contra
// un índice en vez de rechazar la fila con un mensaje legible.
package texto

import "strings"

// Canonizar recorta los bordes y reduce toda corrida de espacios interna a uno
// solo, sin tocar la caja ni las tildes: es el texto tal como se va a GUARDAR
// y mostrar.
//
// Recortar los bordes no alcanza, y el caso no es rebuscado: «Educación
// Física» y «Educación  Física» se imprimen idénticas en cualquier pantalla y
// son dos textos distintos para la base. Un espacio de más lo produce una celda
// de planilla, un copiado de un PDF o un dedo que rebotó, y es invisible tanto
// para quien lo carga como para quien lo revisa después.
//
// El conjunto de qué cuenta como espacio está escrito a mano, y no se usa
// strings.Fields, por una razón concreta: Fields parte por CUALQUIER espacio
// Unicode y Postgres no. El `\s` de su regexp no matchea el espacio duro
// (U+00A0), así que con Fields esta función colapsaba un texto que la base
// dejaba intacto — y las dos definiciones tienen que dar lo mismo o los índices
// únicos dejan de coincidir con el chequeo previo. Lo encontró el test de
// paridad contra Postgres, no una lectura del código.
//
// El conjunto incluye el espacio duro a propósito: es el que viaja al copiar
// desde una página web y el más difícil de ver, porque se imprime igual que uno
// común. Los espacios tipográficos raros (U+2000 y compañía) quedan afuera de
// los dos lados: no aparecen en los datos de una institución, y lo que importa
// no es cubrirlos sino que Go y la base opinen lo mismo sobre ellos.
func Canonizar(s string) string {
	return strings.Join(strings.FieldsFunc(s, esEspacio), " ")
}

// esEspacio es el conjunto que `clave_texto()` traduce a un espacio común. Si
// se toca acá, se toca también en la migración 011.
func esEspacio(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\v', '\f', '\r', '\u00a0':
		return true
	}
	return false
}

// reemplazosDeTilde son los mismos siete caracteres que traduce `clave_texto()`
// en la base: las cinco vocales acentuadas más ü y ñ cubren el castellano.
//
// Se resuelve con un reemplazo explícito y NO con unaccent(): esa función de
// Postgres vive en una extensión, depende de un diccionario y por eso no es
// IMMUTABLE, así que no se puede usar en un índice. Al no poder usarla de aquel
// lado, tampoco se usa de éste: las dos formas tienen que coincidir.
var reemplazosDeTilde = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
)

// Clave es cómo se comparan dos textos para decidir si nombran LA MISMA cosa:
// canonizado, en minúsculas y sin tildes.
//
// Es una pregunta distinta de «¿se escriben igual?». «Matemática» y
// «MATEMATICA» se escriben distinto y son la misma materia; el sistema tiene
// que tratarlas como una sola o termina con las reservas y los docentes de esa
// materia partidos en dos mitades que nadie ve juntas.
func Clave(s string) string {
	return reemplazosDeTilde.Replace(strings.ToLower(Canonizar(s)))
}

// SonElMismo responde la pregunta directamente, para quien no necesita la clave
// sino la comparación.
func SonElMismo(a, b string) bool {
	return Clave(a) == Clave(b)
}
