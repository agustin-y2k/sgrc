//go:build integration

package infrastructure

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// Deshacer una baja, contra Postgres real.
//
// Va acá y no en application porque lo que decide si una reactivación entra no
// es el código: son los tres índices únicos PARCIALES de `equipo` y el de
// `carro`, que excluyen a los dados de baja. Esa exclusión es lo que hace que
// retirar algo libere su nombre —que es deseado— y también lo único que puede
// impedir traerlo de vuelta, si alguien se quedó con lo que liberó.
//
// Un test con un fake diría que la reactivación funciona aunque los índices no
// existieran, y el caso que importa es justamente el que ellos rechazan.

func ahoraDeTest() time.Time {
	return time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
}

// El zócalo: «PC 7 del Carro 1» se liberó al dar de baja la máquina, y otra se
// lo llevó. El error tiene que ser el mismo que da el alta, porque lo que hay
// que hacer es lo mismo: elegir otro identificador.
func TestReactivar_ElIdentificadorSeLoLlevoOtro(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro, err := domain.NuevoCarro(NuevoID(), "Carro de reactivación", "")
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearCarro(ctx, carro); err != nil {
		t.Fatalf("creando el carro: %v", err)
	}

	vieja, err := domain.NuevoEquipoDeCarro(NuevoID(), carro.ID, 7, "SERIE-VIEJA-1", false, ahoraDeTest())
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearEquipo(ctx, vieja); err != nil {
		t.Fatalf("creando la máquina original: %v", err)
	}

	// Se da de baja, y eso libera el 7.
	if err := vieja.DarDeBaja(ahoraDeTest()); err != nil {
		t.Fatalf("dando de baja: %v", err)
	}
	if err := repo.GuardarEquipo(ctx, vieja); err != nil {
		t.Fatalf("guardando la baja: %v", err)
	}

	// Otra máquina ocupa el zócalo — que es exactamente para lo que se liberó.
	nueva, err := domain.NuevoEquipoDeCarro(NuevoID(), carro.ID, 7, "SERIE-NUEVA-1", false, ahoraDeTest())
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearEquipo(ctx, nueva); err != nil {
		t.Fatalf("la máquina de reemplazo tenía que entrar en el zócalo liberado: %v", err)
	}

	// Y ahora la vieja no puede volver a ese lugar.
	if err := vieja.Reactivar(); err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	err = repo.GuardarEquipo(ctx, vieja)

	if !errors.Is(err, application.ErrIdentificadorDuplicado) {
		t.Fatalf("esperaba ErrIdentificadorDuplicado, obtuve %v", err)
	}
}

// El número de serie es el otro que se libera, y el que más cuesta ver: no se
// elige, viene de la etiqueta de fábrica, así que si está ocupado es porque la
// misma máquina se cargó dos veces.
func TestReactivar_ElNumeroDeSerieSeLoLlevoOtro(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro, err := domain.NuevoCarro(NuevoID(), "Carro de series", "")
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearCarro(ctx, carro); err != nil {
		t.Fatalf("creando el carro: %v", err)
	}

	vieja, err := domain.NuevoEquipoDeCarro(NuevoID(), carro.ID, 11, "5CD1234ABC", false, ahoraDeTest())
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearEquipo(ctx, vieja); err != nil {
		t.Fatalf("creando la máquina original: %v", err)
	}
	if err := vieja.DarDeBaja(ahoraDeTest()); err != nil {
		t.Fatalf("dando de baja: %v", err)
	}
	if err := repo.GuardarEquipo(ctx, vieja); err != nil {
		t.Fatalf("guardando la baja: %v", err)
	}

	// Otro zócalo, misma serie: la cargaron de nuevo creyendo que era nueva.
	otra, err := domain.NuevoEquipoDeCarro(NuevoID(), carro.ID, 12, "5cd1234abc", false, ahoraDeTest())
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearEquipo(ctx, otra); err != nil {
		t.Fatalf("la serie estaba liberada, tenía que entrar: %v", err)
	}

	if err := vieja.Reactivar(); err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	err = repo.GuardarEquipo(ctx, vieja)

	if !errors.Is(err, application.ErrNumeroSerieDuplicado) {
		t.Fatalf("esperaba ErrNumeroSerieDuplicado, obtuve %v", err)
	}
}

// Un equipo suelto se identifica por su nombre, y la comparación ignora tildes
// y mayúsculas: «proyector 1» ocupa el lugar de «Proyector 1».
func TestReactivar_ElNombreSueltoSeLoLlevoOtro(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	viejo, err := domain.NuevoEquipoSuelto(NuevoID(), "PROYECTOR", "Proyector del SUM", "", true, ahoraDeTest())
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearEquipo(ctx, viejo); err != nil {
		t.Fatalf("creando el proyector original: %v", err)
	}
	if err := viejo.DarDeBaja(ahoraDeTest()); err != nil {
		t.Fatalf("dando de baja: %v", err)
	}
	if err := repo.GuardarEquipo(ctx, viejo); err != nil {
		t.Fatalf("guardando la baja: %v", err)
	}

	nuevo, err := domain.NuevoEquipoSuelto(NuevoID(), "PROYECTOR", "proyector del sum", "", true, ahoraDeTest())
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearEquipo(ctx, nuevo); err != nil {
		t.Fatalf("el nombre estaba liberado, tenía que entrar: %v", err)
	}

	if err := viejo.Reactivar(); err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	err = repo.GuardarEquipo(ctx, viejo)

	if !errors.Is(err, application.ErrNombreDeEquipoDuplicado) {
		t.Fatalf("esperaba ErrNombreDeEquipoDuplicado, obtuve %v", err)
	}
}

// El camino feliz: nadie tocó nada mientras el equipo estuvo afuera, así que
// vuelve tal como estaba.
func TestReactivar_ConTodoLibre_Vuelve(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro, err := domain.NuevoCarro(NuevoID(), "Carro que vuelve", "")
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearCarro(ctx, carro); err != nil {
		t.Fatalf("creando el carro: %v", err)
	}

	pc, err := domain.NuevoEquipoDeCarro(NuevoID(), carro.ID, 3, "SERIE-VUELVE-1", false, ahoraDeTest())
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearEquipo(ctx, pc); err != nil {
		t.Fatalf("creando el equipo: %v", err)
	}
	if err := pc.DarDeBaja(ahoraDeTest()); err != nil {
		t.Fatalf("dando de baja: %v", err)
	}
	if err := repo.GuardarEquipo(ctx, pc); err != nil {
		t.Fatalf("guardando la baja: %v", err)
	}

	if err := pc.Reactivar(); err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.GuardarEquipo(ctx, pc); err != nil {
		t.Fatalf("con el zócalo y la serie libres tenía que volver: %v", err)
	}

	vuelto, err := repo.BuscarEquipoPorID(ctx, pc.ID)
	if err != nil {
		t.Fatalf("releyendo el equipo: %v", err)
	}
	if vuelto.DadoDeBaja {
		t.Error("el equipo tenía que quedar en circulación")
	}
	if vuelto.FechaBaja != nil {
		t.Errorf("la fecha de baja tenía que quedar nula, quedó %v", vuelto.FechaBaja)
	}
}

// El nombre de un carro se libera igual que el de un equipo suelto, y con la
// misma comparación.
func TestReactivarCarro_ElNombreSeLoLlevoOtro(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	viejo, err := domain.NuevoCarro(NuevoID(), "Carro Reemplazado", "")
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearCarro(ctx, viejo); err != nil {
		t.Fatalf("creando el carro original: %v", err)
	}
	if err := viejo.DarDeBaja(ahoraDeTest()); err != nil {
		t.Fatalf("dando de baja: %v", err)
	}
	if err := repo.GuardarCarro(ctx, viejo); err != nil {
		t.Fatalf("guardando la baja: %v", err)
	}

	nuevo, err := domain.NuevoCarro(NuevoID(), "carro reemplazado", "el que lo reemplaza")
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearCarro(ctx, nuevo); err != nil {
		t.Fatalf("el nombre estaba liberado, tenía que entrar: %v", err)
	}

	if err := viejo.Reactivar(); err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	err = repo.GuardarCarro(ctx, viejo)

	if !errors.Is(err, application.ErrNombreCarroDuplicado) {
		t.Fatalf("esperaba ErrNombreCarroDuplicado, obtuve %v", err)
	}
}

// ListarCarros con los retirados es lo único que hace alcanzable un carro dado
// de baja: sin esto no hay forma de llegar a reactivarlo.
func TestListarCarros_IncluirRetirados(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	vivo, err := domain.NuevoCarro(NuevoID(), "Carro en circulación", "")
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearCarro(ctx, vivo); err != nil {
		t.Fatalf("creando el carro vivo: %v", err)
	}

	retirado, err := domain.NuevoCarro(NuevoID(), "Carro retirado", "")
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearCarro(ctx, retirado); err != nil {
		t.Fatalf("creando el carro a retirar: %v", err)
	}
	if err := retirado.DarDeBaja(ahoraDeTest()); err != nil {
		t.Fatalf("dando de baja: %v", err)
	}
	if err := repo.GuardarCarro(ctx, retirado); err != nil {
		t.Fatalf("guardando la baja: %v", err)
	}

	soloVivos, err := repo.ListarCarros(ctx, false)
	if err != nil {
		t.Fatalf("listando: %v", err)
	}
	for _, c := range soloVivos {
		if c.ID == retirado.ID {
			t.Fatal("el carro retirado no tenía que aparecer en el listado habitual")
		}
	}

	todos, err := repo.ListarCarros(ctx, true)
	if err != nil {
		t.Fatalf("listando con retirados: %v", err)
	}
	encontrado := false
	for _, c := range todos {
		if c.ID == retirado.ID {
			encontrado = true
		}
	}
	if !encontrado {
		t.Error("el carro retirado tenía que aparecer al pedir los retirados")
	}
}
