// Package http expone las rutas Fiber de inventory — ver
// docs/08-api-spec.yaml para el contrato completo de cada endpoint.
package http

import (
	"time"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// ── Requests ────────────────────────────────────────────────────────────

type crearCarroRequest struct {
	Nombre      string `json:"nombre"`
	Descripcion string `json:"descripcion"`
}

type editarCarroRequest struct {
	Nombre      *string `json:"nombre,omitempty"`
	Descripcion *string `json:"descripcion,omitempty"`
}

type crearEquipoDeCarroRequest struct {
	Identificador     int    `json:"identificador"`
	NumeroSerie       string `json:"numeroSerie"`
	Freezado          bool   `json:"freezado"`
	CPU               string `json:"cpu"`
	RAM               string `json:"ram"`
	SistemaOperativo  string `json:"sistemaOperativo"`
	SoftwareInstalado string `json:"softwareInstalado"`
}

// crearEquipoSueltoRequest: algo prestable que no es una computadora de un carro
// — un proyector, un cargador, una notebook suelta.
type crearEquipoSueltoRequest struct {
	// Tipo es texto libre: la lista de cosas que presta una escuela no es la
	// misma que la de otra.
	Tipo   string `json:"tipo"`
	Nombre string `json:"nombre"`
	// NumeroSerie es opcional acá y obligatorio en una computadora de carro:
	// un cargador no trae ninguna. Ausente o vacío se guarda como NULL.
	NumeroSerie string `json:"numeroSerie,omitempty"`
	// Reservable: si aparece en la lista de equipos libres al reservar. El
	// proyector sí; un cargador se presta en el momento.
	Reservable bool `json:"reservable"`
	// EsComputadora habilita la ficha técnica de abajo y las cuentas de
	// acceso. Ausente es false: lo que se presta fuera del laboratorio en su
	// mayoría no es una computadora.
	EsComputadora bool `json:"esComputadora,omitempty"`
	// Los cinco de la ficha técnica, todos opcionales incluso en una
	// computadora: se cargan con lo que se sepa.
	Freezado          bool   `json:"freezado,omitempty"`
	CPU               string `json:"cpu,omitempty"`
	RAM               string `json:"ram,omitempty"`
	SistemaOperativo  string `json:"sistemaOperativo,omitempty"`
	SoftwareInstalado string `json:"softwareInstalado,omitempty"`
}

type editarEquipoRequest struct {
	CarroID           *string `json:"carroId,omitempty"`
	Freezado          *bool   `json:"freezado,omitempty"`
	CPU               *string `json:"cpu,omitempty"`
	RAM               *string `json:"ram,omitempty"`
	SistemaOperativo  *string `json:"sistemaOperativo,omitempty"`
	SoftwareInstalado *string `json:"softwareInstalado,omitempty"`
	Tipo              *string `json:"tipo,omitempty"`
	Nombre            *string `json:"nombre,omitempty"`
	Reservable        *bool   `json:"reservable,omitempty"`
	EsComputadora     *bool   `json:"esComputadora,omitempty"`
	// Cadena vacía borra el número de serie; solo se acepta fuera de un carro.
	NumeroSerie *string `json:"numeroSerie,omitempty"`

	// Estado NO se edita acá y este campo existe solo para poder decirlo. El
	// estado dispara la cascada que cancela reservas (RF-03.8) y por eso vive
	// en su propia ruta, `PATCH /equipos/{id}/estado`. Sin este campo, mandarlo
	// en este cuerpo devolvía 200 y se descartaba en silencio: la respuesta
	// decía que salió bien y la máquina seguía como estaba.
	Estado *string `json:"estado,omitempty"`
}

type cambiarEstadoEquipoRequest struct {
	Estado string  `json:"estado"` // DISPONIBLE | EN_MANTENIMIENTO | FUERA_DE_SERVICIO
	Motivo *string `json:"motivo,omitempty"`
}

type crearIncidenciaRequest struct {
	EquipoID    string `json:"equipoId"`
	Descripcion string `json:"descripcion"`
	Gravedad    string `json:"gravedad"` // LEVE | MODERADA | GRAVE
	// Categoria es opcional: quien reporta la falla no siempre sabe qué es.
	// Se completa después, cuando alguien pudo diagnosticarla.
	Categoria string `json:"categoria,omitempty"`
}

type editarIncidenciaRequest struct {
	Estado                *string `json:"estado,omitempty"`
	MarcarEnviadaASoporte bool    `json:"marcarEnviadaASoporte"`
	// Categoria: mandarla vacía la devuelve a "sin clasificar"; omitirla no
	// la toca.
	Categoria *string `json:"categoria,omitempty"`
}

// ── Responses ───────────────────────────────────────────────────────────

type carroResponse struct {
	ID          string `json:"id"`
	Nombre      string `json:"nombre"`
	Descripcion string `json:"descripcion,omitempty"`
}

func toCarroResponse(c *domain.Carro) carroResponse {
	return carroResponse{ID: c.ID, Nombre: c.Nombre, Descripcion: c.Descripcion}
}

type equipoResponse struct {
	ID string `json:"id"`
	// Los tres pueden faltar: un proyector no está en ningún
	// carro, no es "PC 3" y puede no traer número de serie.
	CarroID       string `json:"carroId,omitempty"`
	Identificador int    `json:"identificador,omitempty"`
	NumeroSerie   string `json:"numeroSerie,omitempty"`
	// Etiqueta es cómo se lo nombra en cualquier pantalla: "PC 3" o
	// "Proyector Epson".
	Etiqueta          string     `json:"etiqueta"`
	Tipo              string     `json:"tipo"`
	Nombre            string     `json:"nombre,omitempty"`
	Reservable        bool       `json:"reservable"`
	EsComputadora     bool       `json:"esComputadora"`
	Freezado          bool       `json:"freezado"`
	CPU               string     `json:"cpu,omitempty"`
	RAM               string     `json:"ram,omitempty"`
	SistemaOperativo  string     `json:"sistemaOperativo,omitempty"`
	SoftwareInstalado string     `json:"softwareInstalado,omitempty"`
	Estado            string     `json:"estado"`
	DadoDeBaja        bool       `json:"dadoDeBaja"`
	FechaBaja         *time.Time `json:"fechaBaja,omitempty"`
	FechaAlta         time.Time  `json:"fechaAlta"`
	// TieneCuentas dice si hay algo que ver detrás de "Cómo entrar"
	// (RF-03.22). La pantalla lo usa para no ofrecerle a un docente un panel
	// vacío en un cargador; un Admin ve el botón igual, porque es el único
	// camino para anotar la primera cuenta.
	TieneCuentas bool `json:"tieneCuentas"`
}

func toEquipoResponse(pc *domain.Equipo) equipoResponse {
	return equipoResponse{
		ID: pc.ID, CarroID: pc.CarroID, Identificador: pc.Identificador, NumeroSerie: pc.NumeroSerie,
		Etiqueta: pc.Etiqueta(), Tipo: pc.Tipo, Nombre: pc.Nombre, Reservable: pc.Reservable,
		EsComputadora: pc.EsComputadora, Freezado: pc.Freezado, CPU: pc.CPU, RAM: pc.RAM, SistemaOperativo: pc.SistemaOperativo,
		SoftwareInstalado: pc.SoftwareInstalado, Estado: string(pc.Estado),
		DadoDeBaja: pc.DadoDeBaja, FechaBaja: pc.FechaBaja, FechaAlta: pc.FechaAlta,
		TieneCuentas: pc.TieneCuentas,
	}
}

type cascadaResponse struct {
	ReservasCanceladas  int `json:"reservasCanceladas"`
	DocentesNotificados int `json:"docentesNotificados"`
}

func toCascadaResponse(r *application.ResultadoCascada) cascadaResponse {
	return cascadaResponse{ReservasCanceladas: r.ReservasCanceladas, DocentesNotificados: r.DocentesNotificados}
}

type incidenciaResponse struct {
	ID           string  `json:"id"`
	EquipoID     string  `json:"equipoId"`
	ReportadoPor *string `json:"reportadoPor,omitempty"`
	Descripcion  string  `json:"descripcion"`
	// Categoria vacía significa que todavía no se diagnosticó.
	Categoria          string     `json:"categoria,omitempty"`
	Gravedad           string     `json:"gravedad"`
	Fecha              time.Time  `json:"fecha"`
	EnviadoASoporte    bool       `json:"enviadoASoporte"`
	FechaEnvioASoporte *time.Time `json:"fechaEnvioASoporte,omitempty"`
	Estado             string     `json:"estado"`
}

func toIncidenciaResponse(i *domain.Incidencia) incidenciaResponse {
	return incidenciaResponse{
		ID: i.ID, EquipoID: i.EquipoID, ReportadoPor: i.ReportadoPor, Descripcion: i.Descripcion,
		Categoria: i.Categoria,
		Gravedad:  string(i.Gravedad), Fecha: i.Fecha, EnviadoASoporte: i.EnviadoASoporte,
		FechaEnvioASoporte: i.FechaEnvioASoporte, Estado: string(i.Estado),
	}
}

// ── Cuentas de usuario de cada equipo (RF-03.22) ────────────────────────

type cuentaRequest struct {
	Usuario string `json:"usuario"`
	// Clase es texto libre —Local, Microsoft, Linux, Google…—: la lista de
	// dónde puede vivir una cuenta no es la misma en cada institución. El
	// formulario sugiere las ya cargadas.
	Clase string `json:"clase"`
	// COMUN | ADMINISTRADOR. Se detalla siempre, aunque la contraseña no se
	// muestre.
	Privilegio string `json:"privilegio"`
	// PUBLICA | SOLO_ADMIN. Decide a quién se le revela la CONTRASEÑA, y la
	// marca un Admin cuenta por cuenta: es independiente del privilegio.
	Visibilidad string `json:"visibilidad"`
	// TienePassword es si la cuenta pide contraseña. Junto con Password da los
	// tres estados: libre, con contraseña anotada, y con contraseña que no
	// sabemos (TienePassword=true y Password vacía).
	TienePassword bool   `json:"tienePassword"`
	Password      string `json:"password,omitempty"`
	Notas         string `json:"notas,omitempty"`
}

type editarCuentaRequest struct {
	Usuario       *string `json:"usuario,omitempty"`
	Clase         *string `json:"clase,omitempty"`
	Privilegio    *string `json:"privilegio,omitempty"`
	Visibilidad   *string `json:"visibilidad,omitempty"`
	TienePassword *bool   `json:"tienePassword,omitempty"`
	Notas         *string `json:"notas,omitempty"`
	// Ausente deja la contraseña que estaba; cadena vacía borra la anotada, que
	// es distinto: la cuenta puede seguir pidiendo contraseña y nosotros pasar
	// a no saberla.
	Password *string `json:"password,omitempty"`
}

// cuentaResponse nunca lleva la contraseña, ni siquiera para un Admin: se pide
// aparte, de a una, y esa petición es la que queda auditada.
type cuentaResponse struct {
	ID         string `json:"id"`
	EquipoID   string `json:"equipoId"`
	Usuario    string `json:"usuario"`
	Clase      string `json:"clase"`
	Privilegio string `json:"privilegio"`
	// Visibilidad la ven todos: saber que una contraseña es reservada explica
	// por qué no aparece el botón, en vez de que la pantalla parezca rota.
	Visibilidad   string `json:"visibilidad"`
	TienePassword bool   `json:"tienePassword"`
	// HayPasswordParaVer distingue el tercer estado: la cuenta pide contraseña
	// pero no la tenemos anotada, así que no hay nada que revelar.
	HayPasswordParaVer bool `json:"hayPasswordParaVer"`
	// PuedeVerLaPassword lo resuelve el servidor para QUIEN pidió esta lista.
	// El frontend solo dibuja el botón; no decide.
	PuedeVerLaPassword bool   `json:"puedeVerLaPassword"`
	Notas              string `json:"notas,omitempty"`
}

func toCuentaResponse(c application.CuentaVisible) cuentaResponse {
	return cuentaResponse{
		ID:                 c.ID,
		EquipoID:           c.EquipoID,
		Usuario:            c.Usuario,
		Clase:              c.Clase,
		Privilegio:         string(c.Privilegio),
		Visibilidad:        string(c.Visibilidad),
		TienePassword:      c.TienePassword,
		HayPasswordParaVer: c.HayPasswordParaVer,
		PuedeVerLaPassword: c.PuedeVerLaPassword,
		Notas:              c.Notas,
	}
}
