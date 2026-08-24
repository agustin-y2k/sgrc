// Espeja internal/inventory/interfaces/http/dto.go — la fuente de verdad es
// ese archivo (ver la nota en lib/api-client.ts sobre por qué no se confía
// ciegamente en docs/08-api-spec.yaml).

export type EstadoEquipo = "DISPONIBLE" | "EN_MANTENIMIENTO" | "FUERA_DE_SERVICIO"

export type Carro = {
  id: string
  nombre: string
  descripcion?: string
}

export type Equipo = {
  id: string
  /**
   * Los tres pueden faltar: una institución también presta proyectores,
   * cargadores y notebooks sueltas, que no están en ningún carro, no son "PC
   * 3" y pueden no traer número de serie.
   */
  carroId?: string
  identificador?: number
  numeroSerie?: string
  /** Cómo se lo nombra en cualquier pantalla: "PC 3" o "Proyector Epson". */
  etiqueta: string
  /** Texto libre: la lista de cosas que presta una escuela no es la de otra. */
  tipo: string
  nombre?: string
  /** Si aparece en la lista de equipos libres al reservar. */
  reservable: boolean
  freezado: boolean
  cpu?: string
  ram?: string
  sistemaOperativo?: string
  softwareInstalado?: string
  estado: EstadoEquipo
  dadoDeBaja: boolean
  fechaBaja?: string
  fechaAlta: string
}

/** RF-03.5 — una falla reportada sobre un equipo. */
export type GravedadIncidencia = "LEVE" | "MODERADA" | "GRAVE"

/**
 * El backend NO impone una máquina de estados acá (a diferencia de Usuario o
 * Equipo): valida que el valor sea uno de los cuatro y nada más, así que
 * cualquier estado puede pasar a cualquier otro.
 */
export type EstadoIncidencia =
  "ABIERTA" | "EN_REPARACION" | "ENVIADA_A_SOPORTE" | "RESUELTA"

export type Incidencia = {
  id: string
  equipoId: string
  reportadoPor?: string
  descripcion: string
  /** Qué tipo de falla es. Vacía mientras no se haya podido diagnosticar. */
  categoria?: string
  gravedad: GravedadIncidencia
  /** ISO 8601 */
  fecha: string
  enviadoASoporte: boolean
  fechaEnvioASoporte?: string
  estado: EstadoIncidencia
}

// Los listados del backend responden { data: [...] }.
export type RespuestaLista<T> = { data: T[] }

/**
 * RF-03.11 — una licencia de software con vencimiento periódico, instalada en
 * un equipo puntual.
 */
export type EstadoLicencia = "SIN_FECHA" | "VENCIDA" | "POR_VENCER" | "VIGENTE"

export type Licencia = {
  id: string
  equipoId: string
  nombre: string
  /** Cuánto dura una renovación. Es el paso del botón "Renovar". */
  diasDuracion: number
  /** Con cuánta antelación avisar. 0 = avisar recién el día que vence. */
  diasAviso: number

  /** Ausente significa "a verificar", NO "no vence nunca". */
  fechaVencimiento?: string
  /** Cuándo se renovó de verdad, que puede no ser cuándo se cargó. */
  ultimaRenovacion?: string

  /** Quién y cuándo lo escribió en el sistema. */
  vencimientoFijadoPor?: string
  vencimientoFijadoEn?: string

  /**
   * Derivados: no están en la base, los calcula el backend contra el día de
   * hoy.
   */
  diasRestantes?: number
  estado: EstadoLicencia

  /** Ubicación. Solo viene en el listado general, no en el de un equipo. */
  /** Cómo se nombra el equipo: "PC 3" o "Notebook chica". */
  etiqueta?: string
  /** 0 en un equipo suelto; el carro, vacío. Lo que se muestra es `etiqueta`. */
  identificador?: number
  carroId?: string
  carroNombre?: string
  equipoDadoDeBaja?: boolean
}

/** Las tres formas de declarar el vencimiento. */
export type VencimientoDeclarado = {
  /** "La renové el martes" → esa fecha + diasDuracion. */
  renovadaEl?: string
  /** "Quedan 12 días" → hoy + 12. Es lo que muestra la máquina. */
  quedanDias?: number
  /** "Vence el 3 de septiembre" → esa fecha, tal cual. */
  venceEl?: string
}

/**
 * `equiposQueYaLaTenian` no es un error: marcar las diez Equipos del carro
 * cuando ocho ya estaban cargadas tiene que funcionar.
 */
export type AltaMasivaLicencias = {
  creadas: Licencia[]
  equiposQueYaLaTenian?: string[]
}

/**
 * `sinFechaPrevia` son las que no se pudieron renovar porque todavía no
 * tienen vencimiento cargado: renovar mueve un contador que ya existe, y
 * cargar la fecha por primera vez se hace editando la licencia, donde hay que
 * decir cómo se sabe.
 */
export type RenovacionLicencias = {
  renovadas: Licencia[]
  sinFechaPrevia?: string[]
}

/** Cómo se nombra cada estado en pantalla. */
export const ETIQUETA_ESTADO_EQUIPO: Record<EstadoEquipo, string> = {
  DISPONIBLE: "Disponible",
  EN_MANTENIMIENTO: "En mantenimiento",
  FUERA_DE_SERVICIO: "Fuera de servicio",
}

/**
 * RF-03.21 — la marca que dice que un equipo es preferente para una materia.
 */
export type PreferenciaDeEquipo = {
  id: string
  equipoId: string
  materiaNombre: string
  /** Ausente = toda materia con ese nombre, en cualquier curso. */
  anio?: number
  /** Ausente = todas las divisiones de ese año. Nunca viene sin `anio`. */
  division?: string
  /** 1 es la más fuerte. */
  prioridad: number
  /** La frase ya armada: "Dibujo Técnico de 3°B". La resuelve el servidor. */
  alcance: string
}

/** Igual que el alta de licencias: el lote se procesa y se informa qué se salteó. */
export type AltaDePreferencias = {
  creadas: PreferenciaDeEquipo[]
  equiposQueYaLaTenian?: string[]
}

// ── Cuentas de usuario de cada equipo (RF-03.22) ────────────────────────

export type PrivilegioDeCuenta = "COMUN" | "ADMINISTRADOR"
export type VisibilidadDeCuenta = "PUBLICA" | "SOLO_ADMIN"

export type CuentaDeEquipo = {
  id: string
  equipoId: string
  usuario: string
  /** Texto libre: Local, Microsoft, Linux, Google… lo que esa escuela tenga. */
  clase: string
  privilegio: PrivilegioDeCuenta
  /** Quién puede ver la CONTRASEÑA. La cuenta en sí se lista siempre. */
  visibilidad: VisibilidadDeCuenta
  /** Si la cuenta pide contraseña para entrar. */
  tienePassword: boolean
  /**
   * Si además la tenemos anotada. Junto con `tienePassword` da los tres
   * estados: libre, anotada, y "pide una que no sabemos".
   */
  hayPasswordParaVer: boolean
  /** Lo resuelve el servidor para quien pidió la lista. Acá solo se dibuja. */
  puedeVerLaPassword: boolean
  notas?: string
}

export type CuentaRequest = {
  usuario: string
  clase: string
  privilegio: PrivilegioDeCuenta
  visibilidad: VisibilidadDeCuenta
  tienePassword: boolean
  password?: string
  notas?: string
}
