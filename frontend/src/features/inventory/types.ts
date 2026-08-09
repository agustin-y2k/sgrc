// Espeja internal/inventory/interfaces/http/dto.go — la fuente de verdad
// es ese archivo (ver la nota en lib/api-client.ts sobre por qué no se
// confía ciegamente en docs/08-api-spec.yaml).

export type EstadoEquipo = "DISPONIBLE" | "EN_MANTENIMIENTO" | "FUERA_DE_SERVICIO"

export type Carro = {
  id: string
  nombre: string
  descripcion?: string
}

export type Equipo = {
  id: string
  /**
   * Los tres pueden faltar desde la 015: la escuela también presta un
   * proyector, cargadores y notebooks sueltas, que no están en ningún carro,
   * no son "PC 3" y pueden no traer número de serie.
   */
  carroId?: string
  identificador?: number
  numeroSerie?: string
  /**
   * Cómo se lo nombra en cualquier pantalla: "PC 3" o "Proyector Epson". Lo
   * resuelve el backend para que la misma cosa no se vea distinta según
   * dónde se la mire, y para que un proyector no salga rotulado "PC 0".
   */
  etiqueta: string
  /** Texto libre: la lista de cosas que presta una escuela no es la de otra. */
  tipo: string
  nombre?: string
  /**
   * Si aparece en la lista de equipos libres al reservar. El proyector sí;
   * un cargador se presta en el momento y nadie planifica con él.
   */
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

/**
 * RF-03.5 — una falla reportada sobre un equipo.
 *
 * Vive acá y no en features/admin porque no es un concepto de
 * administración: el docente que usa el equipo es quien reporta ("Docentes solo
 * pueden reportarlas"), y el Admin es quien después la gestiona.
 */
export type GravedadIncidencia = "LEVE" | "MODERADA" | "GRAVE"

/**
 * El backend NO impone una máquina de estados acá (a diferencia de Usuario o
 * Equipo): valida que el valor sea uno de los cuatro y nada más, así que
 * cualquier estado puede pasar a cualquier otro. La pantalla ofrece el
 * recorrido esperado sin bloquear el resto.
 */
export type EstadoIncidencia = "ABIERTA" | "EN_REPARACION" | "ENVIADA_A_SOPORTE" | "RESUELTA"

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
 * RF-03.11 — una licencia de software con vencimiento periódico, instalada
 * en un equipo puntual.
 *
 * Hay una fila por (Equipo, software): el mismo AutoCAD en las ocho equipos de un
 * carro son ocho licencias. Se modeló así porque el caso a cubrir es
 * justamente el desfasaje —una máquina que quedó sin renovar mientras las
 * demás sí—, y por eso el alta y la renovación son masivas en la pantalla.
 *
 * A diferencia de `Equipo.softwareInstalado`, que es texto libre y lo ve el
 * docente al elegir qué reservar, esto es solo de Admin. Los dos conviven:
 * uno describe qué hay en la máquina, el otro lleva el vencimiento.
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

  /**
   * Ausente significa "a verificar", NO "no vence nunca". Es el estado de
   * una licencia cargada antes de haberse podido sentar delante de la
   * máquina; el job de avisos las ignora y la pantalla las pone primero.
   */
  fechaVencimiento?: string
  /**
   * Cuándo se renovó de verdad, que puede no ser cuándo se cargó. Queda
   * ausente si el vencimiento se fijó por otro camino (por los días que
   * faltan, o escribiendo la fecha): deducirla sería inventar un dato.
   */
  ultimaRenovacion?: string

  /** Quién y cuándo lo escribió en el sistema. */
  vencimientoFijadoPor?: string
  vencimientoFijadoEn?: string

  /**
   * Derivados: no están en la base, los calcula el backend contra el día de
   * hoy. Vienen resueltos para que la pantalla, el correo y el job digan lo
   * mismo — si el navegador los calculara, un reloj corrido mostraría un día
   * distinto del que dispara el aviso.
   *
   * `diasRestantes` negativo = venció hace esos días. Ausente = sin fecha.
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

/**
 * Las tres formas de declarar el vencimiento. Se manda UNA sola: el backend
 * rechaza con 400 si vienen dos, porque darían fechas distintas y elegir
 * cuál gana sería decidir por el Admin.
 *
 * Ninguna de las tres = "todavía no sé", que es un estado válido.
 */
export type VencimientoDeclarado = {
  /** "La renové el martes" → esa fecha + diasDuracion. */
  renovadaEl?: string
  /** "Quedan 12 días" → hoy + 12. Es lo que muestra la máquina. */
  quedanDias?: number
  /** "Vence el 3 de septiembre" → esa fecha, tal cual. */
  venceEl?: string
}

/**
 * `equiposQueYaLaTenian` no es un error: marcar las diez Equipos del carro cuando
 * ocho ya estaban cargadas tiene que funcionar. Por eso el alta responde
 * 201 aunque haya salteado algunas.
 */
export type AltaMasivaLicencias = {
  creadas: Licencia[]
  equiposQueYaLaTenian?: string[]
}

/**
 * `sinFechaPrevia` son las que no se pudieron renovar porque todavía no
 * tienen vencimiento cargado: renovar mueve un contador que ya existe, y
 * cargar la fecha por primera vez se hace editando la licencia, donde hay
 * que decir cómo se sabe.
 */
export type RenovacionLicencias = {
  renovadas: Licencia[]
  sinFechaPrevia?: string[]
}

/**
 * Cómo se nombra cada estado en pantalla. Vive acá y no en cada página
 * porque lo usan el inventario, el panel de administración y los reportes:
 * con una copia por pantalla, renombrar un estado deja las tres diciendo
 * cosas distintas sobre la misma máquina.
 */
export const ETIQUETA_ESTADO_EQUIPO: Record<EstadoEquipo, string> = {
  DISPONIBLE: "Disponible",
  EN_MANTENIMIENTO: "En mantenimiento",
  FUERA_DE_SERVICIO: "Fuera de servicio",
}
