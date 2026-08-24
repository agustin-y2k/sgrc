package http

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// Las cuentas de usuario de cada equipo (RF-03.22), desde la capa HTTP: lo que
// se prueba acá es el reparto de permisos, que es lo que un test de servicio no
// ve porque no pasa por las rutas ni por el middleware.

func appConNotebook(t *testing.T) (*fiber.App, *fakeRepo) {
	t.Helper()
	repo := nuevoFakeRepo()
	repo.equipos["eq1"] = &domain.Equipo{ID: "eq1", Tipo: "NOTEBOOK", Nombre: "Notebook 1"}
	return nuevaAppDeTest(repo), repo
}

func crearCuentaDePrueba(t *testing.T, app *fiber.App, visibilidad, password string) string {
	t.Helper()
	status, cuerpo := pedir(t, app, "POST", "/api/inventory/equipos/eq1/cuentas",
		cuentaRequest{
			Usuario:       "Alumno",
			Clase:         "Local",
			Privilegio:    "COMUN",
			Visibilidad:   visibilidad,
			TienePassword: password != "",
			Password:      password,
		}, "ADMIN")
	if status != fiber.StatusCreated {
		t.Fatalf("esperaba 201 al crear la cuenta, obtuve %d: %s", status, cuerpo)
	}
	var creada cuentaResponse
	if err := json.Unmarshal(cuerpo, &creada); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	return creada.ID
}

// ── El listado lo ve cualquiera; la contraseña, según la marca ───────────

// La cuenta y su privilegio no son el secreto: un docente parado frente a la
// notebook necesita saber con qué usuario entrar.
func TestHTTP_ListarCuentas_UnDocenteLasVe(t *testing.T) {
	app, _ := appConNotebook(t)
	crearCuentaDePrueba(t, app, "SOLO_ADMIN", "SecretaDeLaMaquina")

	status, cuerpo := pedir(t, app, "GET", "/api/inventory/equipos/eq1/cuentas", nil, "DOCENTE")

	if status != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", status, cuerpo)
	}
	var respuesta struct {
		Data []cuentaResponse `json:"data"`
	}
	json.Unmarshal(cuerpo, &respuesta)
	if len(respuesta.Data) != 1 {
		t.Fatalf("esperaba 1 cuenta, obtuve %d", len(respuesta.Data))
	}
	// Pero la marca dice que esta no la puede revelar.
	if respuesta.Data[0].PuedeVerLaPassword {
		t.Error("una cuenta SOLO_ADMIN no la revela un docente")
	}
	if respuesta.Data[0].Usuario != "Alumno" {
		t.Errorf("el usuario se lista igual: %q", respuesta.Data[0].Usuario)
	}
}

// El listado nunca lleva la contraseña, ni para un Admin: se pide de a una
// para que la auditoría distinga "abrió la ficha" de "necesitaba esta clave".
func TestHTTP_ListarCuentas_NoLlevaLaPasswordNiParaUnAdmin(t *testing.T) {
	app, _ := appConNotebook(t)
	crearCuentaDePrueba(t, app, "PUBLICA", "SecretaDeLaMaquina")

	_, cuerpo := pedir(t, app, "GET", "/api/inventory/equipos/eq1/cuentas", nil, "ADMIN")

	if strings.Contains(string(cuerpo), "SecretaDeLaMaquina") {
		t.Fatalf("la contraseña viajó en el listado: %s", cuerpo)
	}
}

// ── Revelar la contraseña ────────────────────────────────────────────────

func TestHTTP_RevelarPassword_PublicaComoDocente_OK(t *testing.T) {
	app, _ := appConNotebook(t)
	id := crearCuentaDePrueba(t, app, "PUBLICA", "SecretaDeLaMaquina")

	status, cuerpo := pedir(t, app, "POST", "/api/inventory/cuentas/"+id+"/password", nil, "DOCENTE")

	if status != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", status, cuerpo)
	}
	var respuesta struct {
		Password string `json:"password"`
	}
	json.Unmarshal(cuerpo, &respuesta)
	if respuesta.Password != "SecretaDeLaMaquina" {
		t.Fatalf("obtuve %q", respuesta.Password)
	}
}

// 403 y no 404: esconder que existe no protegería nada —la cuenta ya se
// lista— y confundiría a quien pregunta por qué no la ve.
func TestHTTP_RevelarPassword_ReservadaComoDocente_403(t *testing.T) {
	app, _ := appConNotebook(t)
	id := crearCuentaDePrueba(t, app, "SOLO_ADMIN", "SecretaDeLaMaquina")

	status, cuerpo := pedir(t, app, "POST", "/api/inventory/cuentas/"+id+"/password", nil, "DOCENTE")

	if status != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d: %s", status, cuerpo)
	}
	if strings.Contains(string(cuerpo), "SecretaDeLaMaquina") {
		t.Fatal("el 403 no puede filtrar la contraseña en el mensaje")
	}
}

func TestHTTP_RevelarPassword_ReservadaComoAdmin_OK(t *testing.T) {
	app, _ := appConNotebook(t)
	id := crearCuentaDePrueba(t, app, "SOLO_ADMIN", "SecretaDeLaMaquina")

	status, _ := pedir(t, app, "POST", "/api/inventory/cuentas/"+id+"/password", nil, "ADMIN")

	if status != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", status)
	}
}

// El tercer estado: la cuenta pide contraseña y no la tenemos anotada.
func TestHTTP_RevelarPassword_SinAnotar_404(t *testing.T) {
	app, _ := appConNotebook(t)
	// TienePassword=true y Password vacía.
	status, cuerpo := pedir(t, app, "POST", "/api/inventory/equipos/eq1/cuentas",
		cuentaRequest{Usuario: "Alumno", Clase: "Local", Privilegio: "COMUN", Visibilidad: "PUBLICA", TienePassword: true}, "ADMIN")
	if status != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d: %s", status, cuerpo)
	}
	var creada cuentaResponse
	json.Unmarshal(cuerpo, &creada)

	if creada.HayPasswordParaVer {
		t.Error("no hay contraseña anotada para ver")
	}
	if !creada.TienePassword {
		t.Error("pero la cuenta sí pide contraseña")
	}

	status, _ = pedir(t, app, "POST", "/api/inventory/cuentas/"+creada.ID+"/password", nil, "ADMIN")
	if status != fiber.StatusNotFound {
		t.Fatalf("esperaba 404, obtuve %d", status)
	}
}

// ── Escribir es solo de Admin ────────────────────────────────────────────

func TestHTTP_CuentasComoDocente_NoPuedeEscribir(t *testing.T) {
	app, _ := appConNotebook(t)
	id := crearCuentaDePrueba(t, app, "PUBLICA", "SecretaDeLaMaquina")
	doc := "DOCENTE"

	casos := []struct {
		nombre string
		metodo string
		ruta   string
		body   any
	}{
		{"crear", "POST", "/api/inventory/equipos/eq1/cuentas", cuentaRequest{
			Usuario: "Otra", Clase: "Local", Privilegio: "COMUN", Visibilidad: "PUBLICA"}},
		{"editar", "PATCH", "/api/inventory/cuentas/" + id, editarCuentaRequest{}},
		{"borrar", "DELETE", "/api/inventory/cuentas/" + id, nil},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			status, _ := pedir(t, app, c.metodo, c.ruta, c.body, doc)
			if status != fiber.StatusForbidden {
				t.Fatalf("esperaba 403, obtuve %d", status)
			}
		})
	}
}

// Cambiar la visibilidad es el caso normal: alguien se da cuenta de que esa
// cuenta no debería ser pública. No hay que volver a mandar la contraseña.
func TestHTTP_EditarCuenta_CambiarVisibilidadSinMandarLaPassword(t *testing.T) {
	app, _ := appConNotebook(t)
	id := crearCuentaDePrueba(t, app, "PUBLICA", "SecretaDeLaMaquina")

	reservada := "SOLO_ADMIN"
	status, cuerpo := pedir(t, app, "PATCH", "/api/inventory/cuentas/"+id,
		editarCuentaRequest{Visibilidad: &reservada}, "ADMIN")
	if status != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", status, cuerpo)
	}

	// La contraseña sigue ahí, y ahora un docente no la ve.
	status, _ = pedir(t, app, "POST", "/api/inventory/cuentas/"+id+"/password", nil, "ADMIN")
	if status != fiber.StatusOK {
		t.Fatalf("el Admin la tenía que seguir viendo, obtuve %d", status)
	}
	status, _ = pedir(t, app, "POST", "/api/inventory/cuentas/"+id+"/password", nil, "DOCENTE")
	if status != fiber.StatusForbidden {
		t.Fatalf("el docente ya no la ve, esperaba 403 y obtuve %d", status)
	}
}

func TestHTTP_CrearCuenta_UsuarioRepetidoEnElMismoEquipo_409(t *testing.T) {
	app, _ := appConNotebook(t)
	crearCuentaDePrueba(t, app, "PUBLICA", "abc")

	status, _ := pedir(t, app, "POST", "/api/inventory/equipos/eq1/cuentas",
		cuentaRequest{Usuario: "alumno", Clase: "Local", Privilegio: "COMUN", Visibilidad: "PUBLICA"}, "ADMIN")

	if status != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d", status)
	}
}

// ── Que todo esto sea OPCIONAL, desde la API ────────────────────────────

// Un equipo sin cuentas responde 200 con una lista vacía, NO un 404. La
// diferencia importa: un 404 haría que la pantalla muestre un error donde lo
// que pasa es que todavía no se anotó nada.
func TestHTTP_Opcional_EquipoSinCuentas_200YListaVacia(t *testing.T) {
	app, _ := appConNotebook(t)

	status, cuerpo := pedir(t, app, "GET", "/api/inventory/equipos/eq1/cuentas", nil, "DOCENTE")

	if status != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", status, cuerpo)
	}
	var respuesta struct {
		Data []cuentaResponse `json:"data"`
	}
	if err := json.Unmarshal(cuerpo, &respuesta); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(respuesta.Data) != 0 {
		t.Fatalf("esperaba ninguna cuenta, obtuve %d", len(respuesta.Data))
	}
	// `data: []` y no `data: null`: un null obliga a cada consumidor a
	// defenderse antes de recorrerlo.
	if !strings.Contains(string(cuerpo), `"data":[]`) {
		t.Errorf("esperaba una lista vacía y no null: %s", cuerpo)
	}
}

// Anotar una cuenta sin contraseña es válido por las dos vías, y el cuerpo lo
// refleja para que la pantalla pueda distinguirlas.
func TestHTTP_Opcional_CuentaSinContrasena(t *testing.T) {
	app, _ := appConNotebook(t)

	casos := []struct {
		nombre         string
		usuario        string
		tienePassword  bool
		esperaTiene    bool
		esperaHayParaV bool
	}{
		{"la cuenta es libre", "kiosco", false, false, false},
		{"pide una que no sabemos", "alumno", true, true, false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			status, cuerpo := pedir(t, app, "POST", "/api/inventory/equipos/eq1/cuentas",
				cuentaRequest{
					Usuario: c.usuario, Clase: "Local", Privilegio: "COMUN",
					Visibilidad: "PUBLICA", TienePassword: c.tienePassword,
				}, "ADMIN")

			if status != fiber.StatusCreated {
				t.Fatalf("esperaba 201, obtuve %d: %s", status, cuerpo)
			}
			var creada cuentaResponse
			json.Unmarshal(cuerpo, &creada)
			if creada.TienePassword != c.esperaTiene {
				t.Errorf("tienePassword: esperaba %v", c.esperaTiene)
			}
			if creada.HayPasswordParaVer != c.esperaHayParaV {
				t.Errorf("hayPasswordParaVer: esperaba %v", c.esperaHayParaV)
			}
		})
	}
}

// El equipo se da de alta sin que nadie pida cuentas: son dos operaciones
// separadas y la segunda puede no ocurrir nunca.
func TestHTTP_Opcional_AltaDeEquipoNoPideCuentas(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	status, cuerpo := pedir(t, app, "POST", "/api/inventory/equipos",
		crearEquipoSueltoRequest{Tipo: "NOTEBOOK", Nombre: "Notebook Dirección"}, "ADMIN")

	if status != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d: %s", status, cuerpo)
	}
}

// Una cuenta mal escrita se contesta con 400 y con el motivo, no con "error
// interno". Las validaciones nacen en el dominio, así que si mapearError no
// las conoce, la pantalla recibe un 500 y quien carga la cuenta no se entera
// de qué le falta.
func TestHTTP_CuentaInvalida_Da400ConElMotivo(t *testing.T) {
	casos := []struct {
		nombre string
		req    cuentaRequest
		dice   string
	}{
		{
			"sin el nombre de la cuenta",
			cuentaRequest{Clase: "Local", Privilegio: "COMUN", Visibilidad: "PUBLICA"},
			"nombre de la cuenta",
		},
		{
			"sin el tipo de cuenta",
			cuentaRequest{Usuario: "Alumno", Privilegio: "COMUN", Visibilidad: "PUBLICA"},
			"de qué tipo es la cuenta",
		},
		{
			"un privilegio que no existe",
			cuentaRequest{Usuario: "Alumno", Clase: "Local", Privilegio: "ROOT", Visibilidad: "PUBLICA"},
			"COMUN o ADMINISTRADOR",
		},
		{
			"una visibilidad que no existe",
			cuentaRequest{Usuario: "Alumno", Clase: "Local", Privilegio: "COMUN", Visibilidad: "TODOS"},
			"PUBLICA o SOLO_ADMIN",
		},
		{
			"una contraseña en una cuenta marcada como libre",
			cuentaRequest{Usuario: "Alumno", Clase: "Local", Privilegio: "COMUN", Visibilidad: "PUBLICA",
				TienePassword: false, Password: "algo"},
			"marcada como libre",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			app, _ := appConNotebook(t)

			status, cuerpo := pedir(t, app, "POST", "/api/inventory/equipos/eq1/cuentas", c.req, "ADMIN")

			if status != fiber.StatusBadRequest {
				t.Fatalf("esperaba 400, obtuve %d: %s", status, cuerpo)
			}
			if !strings.Contains(string(cuerpo), c.dice) {
				t.Errorf("el mensaje tendría que explicar el problema (%q), y dice: %s", c.dice, cuerpo)
			}
		})
	}
}
