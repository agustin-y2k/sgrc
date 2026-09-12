package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/ramiro/sgrc/internal/academic/domain"
)

// RF-02.12 — las tres operaciones que cargan cursos y materias de a muchos en
// vez de de a uno: descargar la estructura de un ciclo, volver a cargarla, y
// copiar las materias de un curso a otros.
//
// Las tres existen por el mismo motivo. Cargar una institución curso por curso
// y materia por materia son cientos de formularios, y el año siguiente son los
// mismos cientos otra vez: la estructura académica se repite entre divisiones
// del mismo año y se repite entre ciclos. Clonar al archivar (RF-02.5) resuelve
// el segundo caso y sólo el segundo, y sólo en el momento exacto de cerrar el
// año.

const (
	// MaxCursosPorImportacion y MaxCursosDestino son topes de sanidad, no
	// reglas de dominio: están lejos de cualquier institución real y cerca de
	// lo que distingue una planilla de cursos de un archivo equivocado.
	MaxCursosPorImportacion = 500
	MaxCursosDestino        = 200
)

// ExportarEstructura devuelve los cursos del ciclo con sus materias, en la
// misma forma que acepta ImportarEstructura. Que las dos hablen la misma forma
// es el punto: lo que se descarga se corrige en una planilla y se vuelve a
// subir, y lo de un ciclo sirve para armar el siguiente.
func (s *Service) ExportarEstructura(ctx context.Context, cicloID string) ([]CursoConMaterias, error) {
	if _, err := s.repo.BuscarCicloPorID(ctx, cicloID); err != nil {
		return nil, err
	}
	return s.repo.ListarEstructuraDeCiclo(ctx, cicloID)
}

// ImportarEstructura crea los cursos y las materias del archivo que todavía no
// existan en el ciclo, y deja intacto todo lo demás: nunca borra ni renombra.
//
// Es la única semántica segura para una operación que se reintenta. Subir dos
// veces el mismo archivo tiene que ser inofensivo, y un archivo al que le falta
// una materia que alguien agregó a mano no puede hacerla desaparecer. La
// contracara —que una materia mal escrita se corrige a mano y no volviendo a
// importar— es el precio, y es el barato de los dos.
func (s *Service) ImportarEstructura(ctx context.Context, cicloID string, cursos []CursoConMaterias) (*ResultadoImportacion, error) {
	ciclo, err := s.repo.BuscarCicloPorID(ctx, cicloID)
	if err != nil {
		return nil, err
	}
	if ciclo.Archivado {
		return nil, ErrCicloArchivado
	}

	limpios, err := limpiarEstructura(cursos)
	if err != nil {
		return nil, err
	}
	if len(limpios) == 0 {
		return nil, ErrSinCursosParaImportar
	}
	if len(limpios) > MaxCursosPorImportacion {
		return nil, ErrImportacionDemasiadoGrande
	}

	resultado, err := s.repo.ImportarEstructura(ctx, cicloID, limpios)
	if err != nil {
		return nil, err
	}
	return &resultado, nil
}

// CopiarMaterias duplica los nombres de materia del curso de origen en cada
// curso de destino, salteando las que el destino ya tenga.
//
// Los destinos tienen que ser del mismo ciclo. No es una limitación técnica:
// copiar hacia otro año es armar el ciclo siguiente, y para eso están el clonado
// del archivado (RF-02.5) y la descarga/carga de la estructura entera, que
// mueven también los cursos. Una copia de materias que cruza ciclos en silencio
// se parece demasiado a las dos y no hace ninguna de las dos.
func (s *Service) CopiarMaterias(ctx context.Context, cursoOrigenID string, cursosDestinoIDs []string) (*ResultadoCopia, error) {
	origen, err := s.repo.BuscarCursoPorID(ctx, cursoOrigenID)
	if err != nil {
		return nil, err
	}

	ciclo, err := s.repo.BuscarCicloPorID(ctx, origen.CicloLectivoID)
	if err != nil {
		return nil, err
	}
	if ciclo.Archivado {
		return nil, ErrCicloArchivado
	}

	destinos, err := s.validarDestinos(ctx, origen, cursosDestinoIDs)
	if err != nil {
		return nil, err
	}

	materias, err := s.repo.ListarMateriasPorCurso(ctx, cursoOrigenID)
	if err != nil {
		return nil, err
	}
	if len(materias) == 0 {
		return nil, ErrCursoOrigenSinMat
	}

	resultado, err := s.repo.CopiarMateriasA(ctx, cursoOrigenID, destinos)
	if err != nil {
		return nil, err
	}
	return &resultado, nil
}

// validarDestinos saca los repetidos y verifica que cada destino exista y
// pertenezca al mismo ciclo que el origen.
func (s *Service) validarDestinos(ctx context.Context, origen *domain.Curso, ids []string) ([]string, error) {
	vistos := make(map[string]bool, len(ids))
	destinos := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == origen.ID {
			return nil, ErrCopiaAlMismoCurso
		}
		if vistos[id] {
			continue
		}
		vistos[id] = true
		destinos = append(destinos, id)
	}
	if len(destinos) == 0 {
		return nil, ErrSinCursosDestino
	}
	if len(destinos) > MaxCursosDestino {
		return nil, ErrDemasiadosDestinos
	}

	// Los ciclos de TODOS los destinos en una sola consulta. Preguntar de a uno
	// eran treinta viajes a la base para contestar algo que un IN contesta
	// entero, y además hacía que el tiempo de la operación dependiera de cuántos
	// cursos se eligieran —justo lo que la copia masiva viene a evitar—.
	ciclos, err := s.repo.CiclosDeCursos(ctx, destinos)
	if err != nil {
		return nil, fmt.Errorf("verificando los cursos de destino: %w", err)
	}
	for _, id := range destinos {
		ciclo, existe := ciclos[id]
		if !existe {
			// Ausente del mapa = no existe. Se devuelve el mismo error que daría
			// buscarlo de a uno, para que quien llama no distinga dos caminos.
			return nil, fmt.Errorf("buscando el curso de destino %s: %w", id, ErrCursoNoEncontrado)
		}
		if ciclo != origen.CicloLectivoID {
			return nil, ErrCopiaEntreCiclos
		}
	}
	return destinos, nil
}

// limpiarEstructura valida y normaliza lo que llegó antes de que toque la base:
// recorta los textos, descarta las materias repetidas dentro de un curso y funde
// las filas que hablan del mismo curso.
//
// Las dos deduplicaciones son por nombre NORMALIZADO —sin acentos ni
// mayúsculas—, igual que compara la base. Una planilla escrita a mano trae
// «Matemática» y «matematica» en dos renglones sin que nadie lo note, y el
// UNIQUE de `materia` es sensible a mayúsculas: sin esto la importación crea las
// dos y el curso queda con la misma materia dos veces.
//
// Fundir en vez de rechazar: dos renglones para el mismo curso es cómo se ve una
// planilla donde alguien partió las materias en dos filas para que entren en la
// pantalla. Rechazar el archivo entero por eso obliga a editarlo a mano para
// decir lo mismo.
func limpiarEstructura(cursos []CursoConMaterias) ([]CursoConMaterias, error) {
	// El orden del archivo se conserva: es el que eligió quien lo armó, y el
	// resumen que devuelve la importación se lee contra él.
	porClave := make(map[string]int, len(cursos))
	limpios := make([]CursoConMaterias, 0, len(cursos))
	materiasVistas := make([]map[string]bool, 0, len(cursos))

	for i, fila := range cursos {
		division, modalidad, err := domain.ValidarCurso(fila.Anio, fila.Division, fila.Modalidad)
		if err != nil {
			// El número de fila es lo único que permite encontrar el renglón malo
			// en una planilla de cien: sin él, el mensaje describe un problema que
			// hay que salir a buscar.
			return nil, fmt.Errorf("fila %d: %w", i+1, err)
		}

		clave := claveDeCurso(fila.Anio, division, modalidad)
		indice, yaEstaba := porClave[clave]
		if !yaEstaba {
			indice = len(limpios)
			porClave[clave] = indice
			limpios = append(limpios, CursoConMaterias{
				Anio: fila.Anio, Division: division, Modalidad: modalidad,
			})
			materiasVistas = append(materiasVistas, make(map[string]bool, len(fila.Materias)))
		}

		for _, nombre := range fila.Materias {
			// Un hueco entre dos comas no es un error que valga frenar el archivo:
			// es cómo se ve una planilla con una coma de más al final de la lista.
			// Lo demás sí se valida y sí frena.
			if strings.TrimSpace(nombre) == "" {
				continue
			}
			limpio, err := domain.ValidarNombreMateria(nombre)
			if err != nil {
				return nil, fmt.Errorf("fila %d: %w", i+1, err)
			}
			norm := domain.NormalizarNombre(limpio)
			if materiasVistas[indice][norm] {
				continue
			}
			materiasVistas[indice][norm] = true
			limpios[indice].Materias = append(limpios[indice].Materias, limpio)
		}
	}
	return limpios, nil
}

// claveDeCurso identifica un curso por lo mismo que lo identifica el índice
// único de la base: la terna normalizada. Dos filas que dicen «4°1» y «4° 1 » son
// el mismo curso.
func claveDeCurso(anio int, division, modalidad string) string {
	return fmt.Sprintf("%d\x00%s\x00%s",
		anio, domain.NormalizarNombre(division), domain.NormalizarNombre(modalidad))
}
