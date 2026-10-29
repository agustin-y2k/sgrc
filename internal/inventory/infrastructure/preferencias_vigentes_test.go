//go:build integration

package infrastructure

import (
	"context"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/inventory/domain"
)

func TestPreferenciasVigentes_ArchivoNoOcultaMarcasSinAplicacion(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	equipo := equipoParaPreferencias(t, repo, "VIGENCIA")
	for _, exacta := range []bool{false, true} {
		for _, caso := range []string{"vigente", "materia-archivada", "curso-archivado", "ciclo-archivado", "espacio-archivado", "otra-archivada", "otra-vigente", "archivo-otro-alcance", "inactivo-sin-archivar"} {
			if exacta && caso == "espacio-archivado" {
				continue
			}
			t.Run(caso+map[bool]string{true: "-exacta", false: "-general"}[exacta], func(t *testing.T) {
				ejecutar := func(sql string, args ...any) {
					t.Helper()
					if _, err := pool.Exec(ctx, sql, args...); err != nil {
						t.Fatal(err)
					}
				}
				ejecutar(`TRUNCATE ciclo_lectivo CASCADE; DELETE FROM equipo_preferencia`)
				ciclo, contenedor, materia := NuevoID(), NuevoID(), NuevoID()
				ejecutar(`INSERT INTO ciclo_lectivo(id,anio,activo) VALUES($1,2026,$2)`, ciclo, caso != "inactivo-sin-archivar")
				if caso == "espacio-archivado" {
					ejecutar(`INSERT INTO espacio(id,ciclo_lectivo_id,nombre) VALUES($1,$2,'Taller')`, contenedor, ciclo)
					ejecutar(`INSERT INTO materia(id,espacio_id,nombre) VALUES($1,$2,'Matemática')`, materia, contenedor)
				} else {
					ejecutar(`INSERT INTO curso(id,ciclo_lectivo_id,anio,division,modalidad) VALUES($1,$2,3,'2','Construcción')`, contenedor, ciclo)
					ejecutar(`INSERT INTO materia(id,curso_id,nombre) VALUES($1,$2,'Matemática')`, materia, contenedor)
				}
				anio, division, modalidad := ptrInt(3), ptrStr("2"), ptrStr("Construcción")
				if !exacta {
					division = nil
				}
				if caso == "espacio-archivado" {
					anio, division, modalidad = nil, nil, nil
				}
				marca, err := domain.NuevaPreferencia(NuevoID(), equipo, "matematica", modalidad, anio, division, 1, exacta)
				if err != nil {
					t.Fatal(err)
				}
				if creada, err := repo.CrearPreferencia(ctx, marca); err != nil || !creada {
					t.Fatalf("marca: %v %v", creada, err)
				}
				archivada := false
				switch caso {
				case "materia-archivada":
					ejecutar(`UPDATE materia SET archivado=true WHERE id=$1`, materia)
					archivada = true
				case "curso-archivado":
					ejecutar(`UPDATE curso SET archivado=true WHERE id=$1`, contenedor)
					archivada = true
				case "ciclo-archivado":
					ejecutar(`UPDATE ciclo_lectivo SET activo=false,archivado=true WHERE id=$1`, ciclo)
					archivada = true
				case "espacio-archivado":
					ejecutar(`UPDATE espacio SET archivado=true WHERE id=$1`, contenedor)
					archivada = true
				}
				if caso == "otra-archivada" || caso == "otra-vigente" || caso == "archivo-otro-alcance" {
					otroCiclo, otroCurso := NuevoID(), NuevoID()
					ejecutar(`INSERT INTO ciclo_lectivo(id,anio,activo,archivado) VALUES($1,2025,false,$2)`, otroCiclo, caso != "otra-vigente")
					otroAnio := 3
					if caso == "archivo-otro-alcance" {
						otroAnio = 4
					}
					ejecutar(`INSERT INTO curso(id,ciclo_lectivo_id,anio,division,modalidad) VALUES($1,$2,$3,'2','Construcción')`, otroCurso, otroCiclo, otroAnio)
					ejecutar(`INSERT INTO materia(id,curso_id,nombre) VALUES($1,$2,'MATEMATICA')`, NuevoID(), otroCurso)
				}
				comprobar := func(sinAplicacion, historica bool) {
					t.Helper()
					marcas, err := repo.ListarPreferenciasHuerfanas(ctx)
					if err != nil {
						t.Fatal(err)
					}
					if !sinAplicacion {
						if len(marcas) != 0 {
							t.Fatalf("prioridad todavía vigente: %+v", marcas)
						}
						return
					}
					if len(marcas) != 1 || marcas[0].ID != marca.ID || marcas[0].CoincideConArchivadas != historica {
						t.Fatalf("sin aplicación: %+v; histórica=%v", marcas, historica)
					}
				}
				comprobar(archivada, archivada)
				n, err := repo.ContarPreferenciasQueDejarianDeAplicar(ctx, materia, "Matemática")
				esperado := 1
				if archivada || caso == "otra-vigente" {
					esperado = 0
				}
				if err != nil || n != esperado {
					t.Fatalf("conteo=%d esperado=%d error=%v", n, esperado, err)
				}
				if !archivada {
					ejecutar(`UPDATE materia SET nombre='Matemática aplicada' WHERE id=$1`, materia)
					comprobar(caso != "otra-vigente", caso == "otra-archivada")
				}
				leida, err := repo.BuscarPreferenciaPorID(ctx, marca.ID)
				if err != nil || leida.Prioridad != 1 || leida.MateriaNombre != "matematica" {
					t.Fatalf("la revisión modificó la marca: %+v %v", leida, err)
				}
			})
		}
	}
}
