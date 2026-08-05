// Package archtest impone en la suite de tests el límite de dominio de
// docs/06-arquitectura.md §1/§3: "ningún paquete importa domain/ de otro
// directamente". Sin este test, la disciplina de límites entre paquetes
// (que es la única razón de ser del "monolito modular" en vez de un
// monolito plano) se erosiona con el tiempo sin que nadie lo note — un
// import de más compila perfecto y nadie lo revisa a mano en cada PR.
package archtest

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

const modulePrefix = "github.com/ramiro/sgrc/internal/"

// paqueteDeDominio matchea imports del tipo
// ".../internal/<paquete>/domain" (o subpaquetes de domain, si alguna vez
// hubiera).
var paqueteDeDominio = regexp.MustCompile(`^` + regexp.QuoteMeta(modulePrefix) + `([^/]+)/domain(/|$)`)

// TestNingunPaqueteImportaDomainAjeno recorre cada .go de internal/ y
// falla si encuentra un import de internal/<X>/domain desde un archivo
// que no pertenece al propio paquete <X> (ni a internal/shared/, que es
// transversal por diseño — ver docs/06-arquitectura.md §2).
func TestNingunPaqueteImportaDomainAjeno(t *testing.T) {
	root := internalDir(t)

	var violaciones []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isGoFile(path) {
			return nil
		}

		paquetePropio := paqueteFeatureDe(root, path)

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}

		for _, imp := range f.Imports {
			importPath := imp.Path.Value
			importPath = importPath[1 : len(importPath)-1] // sacar comillas

			m := paqueteDeDominio.FindStringSubmatch(importPath)
			if m == nil {
				continue
			}
			paqueteAjeno := m[1]

			if paqueteAjeno == paquetePropio {
				continue // un paquete SÍ puede importar su propio domain
			}
			if paquetePropio == "shared" {
				continue // shared es transversal, no tiene domain propio
			}

			violaciones = append(violaciones, path+" importa "+importPath)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("error recorriendo internal/: %v", err)
	}

	if len(violaciones) > 0 {
		sort.Strings(violaciones)
		t.Fatalf("límite de dominio violado (ver docs/06-arquitectura.md §3) — "+
			"ningún paquete debe importar internal/<otro>/domain directamente, "+
			"solo un puerto en su propio application/:\n%s",
			joinLines(violaciones))
	}
}

func isGoFile(path string) bool {
	return filepath.Ext(path) == ".go"
}

// paqueteFeatureDe devuelve el primer segmento de path relativo a
// internal/ (ej: internal/reservation/application/service.go → "reservation").
func paqueteFeatureDe(internalRoot, path string) string {
	rel, err := filepath.Rel(internalRoot, path)
	if err != nil {
		return ""
	}
	// filepath.Rel devuelve algo como "reservation/application/service.go"
	if idx := indexOfSeparator(rel); idx >= 0 {
		return rel[:idx]
	}
	return rel
}

func indexOfSeparator(s string) int {
	for i, r := range s {
		if r == filepath.Separator {
			return i
		}
	}
	return -1
}

// internalDir ubica el directorio internal/ del repo a partir del propio
// archivo de test, sin depender del directorio de trabajo desde el que se
// invoque `go test`.
func internalDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("no se pudo obtener el directorio de trabajo: %v", err)
	}
	// Este archivo vive en internal/shared/archtest — subir dos niveles.
	dir := filepath.Dir(filepath.Dir(wd))
	if filepath.Base(dir) != "internal" {
		t.Fatalf("esperaba terminar en un directorio internal/, llegué a %q", dir)
	}
	return dir
}

func joinLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += "  - " + l + "\n"
	}
	return out
}
