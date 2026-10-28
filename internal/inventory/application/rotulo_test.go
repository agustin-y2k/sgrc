package application

import (
	"context"
	"errors"
	"github.com/agustin-y2k/sgrc/internal/inventory/domain"
	"testing"
	"time"
)

func TestRenumerarEquipo_ConservaIdentidadYSerie(t *testing.T) {
	repo := nuevoFakeRepo()
	pc, err := domain.NuevoEquipoDeCarro("pc", "carro", 4, "SERIE-FISICA", false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	repo.equipos[pc.ID] = pc
	cambios, err := servicioSimple(repo).RenumerarEquipo(context.Background(), pc.ID, 4, 18, "Reemplazo del rótulo")
	if err != nil {
		t.Fatal(err)
	}
	despues := repo.equipos[pc.ID]
	if despues.ID != "pc" || despues.CarroID != "carro" || despues.NumeroSerie != "SERIE-FISICA" || despues.Identificador != 18 {
		t.Fatalf("cambió la identidad: %+v", despues)
	}
	if len(cambios) != 1 || cambios["identificador"].Antes != 4 || cambios["identificador"].Despues != 18 {
		t.Fatalf("auditoría incompleta: %+v", cambios)
	}
	_, err = servicioSimple(repo).RenumerarEquipo(context.Background(), pc.ID, 4, 19, "Formulario desactualizado")
	if !errors.Is(err, ErrRotuloDesactualizado) || despues.Identificador != 18 {
		t.Fatalf("se aceptó un formulario viejo: %v", err)
	}
}

func TestRenumerarEquipo_Restricciones(t *testing.T) {
	for _, tc := range []struct {
		nombre         string
		actual, numero int
		motivo         string
		esperado       error
	}{
		{"cero", 4, 0, "Rótulo nuevo", domain.ErrIdentificadorInvalido},
		{"negativo", 4, -1, "Rótulo nuevo", domain.ErrIdentificadorInvalido},
		{"fuera de rango", 4, 2147483648, "Rótulo nuevo", domain.ErrIdentificadorInvalido},
		{"sin motivo", 4, 5, "  ", ErrMotivoRenumeracion},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			_, err := servicioSimple(nuevoFakeRepo()).RenumerarEquipo(context.Background(), "pc", tc.actual, tc.numero, tc.motivo)
			if !errors.Is(err, tc.esperado) {
				t.Fatalf("esperaba %v, obtuvo %v", tc.esperado, err)
			}
		})
	}
	for _, baja := range []bool{false, true} {
		repo := nuevoFakeRepo()
		pc, _ := domain.NuevoEquipoSuelto("suelto", "PROYECTOR", "Proyector", "", true, time.Now())
		pc.DadoDeBaja = baja
		repo.equipos[pc.ID] = pc
		_, err := servicioSimple(repo).RenumerarEquipo(context.Background(), pc.ID, 1, 2, "Rótulo nuevo")
		esperado := ErrEquipoSinRotulo
		if baja {
			esperado = domain.ErrEquipoYaDadoDeBaja
		}
		if !errors.Is(err, esperado) {
			t.Fatalf("esperaba %v, obtuvo %v", esperado, err)
		}
	}
}
