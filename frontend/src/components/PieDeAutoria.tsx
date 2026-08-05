/**
 * Quién hizo esto y bajo qué licencia se puede usar.
 *
 * La licencia MIT permite casi todo a cambio de una sola cosa: que el aviso
 * de autoría viaje con el software. Este pie lo hace visible donde la gente
 * lo ve, no solo en un LICENSE que nadie abre.
 */

const AUTOR = "Ramiro Agustin Pintos De Nucci"
const ANIO = 2026

/** La inyecta Vite desde package.json (ver vite.config.ts). */
declare const __VERSION__: string

export function PieDeAutoria({ className = "" }: { className?: string }) {
  return (
    <footer
      className={`text-muted-foreground text-center text-xs text-balance ${className}`}
    >
      <p>
        SGRC v{__VERSION__} — software libre bajo licencia{" "}
        <a
          href="https://opensource.org/licenses/MIT"
          target="_blank"
          rel="noreferrer"
          className="hover:text-foreground underline underline-offset-2"
        >
          MIT
        </a>
      </p>
      <p>
        © {ANIO} {AUTOR}
      </p>
    </footer>
  )
}
