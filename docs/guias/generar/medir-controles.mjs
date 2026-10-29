// Coordenadas de los controles reales dentro del mismo encuadre de la captura.
export async function medirControles(pagina, encuadre) {
  return pagina
    .locator(
      "button,input,select,textarea,a,summary,[role=checkbox],[role=button],h1,h2,h3,[data-slot=card-title],[data-slot=badge],[role=alert],p,th"
    )
    .evaluateAll((elementos, marco) => {
      const normalizar = (s) => (s ?? "").replace(/\s+/g, " ").trim()
      return elementos.flatMap((el) => {
        const estilo = getComputedStyle(el)
        const r = el.getBoundingClientRect()
        if (
          !el.getClientRects().length ||
          estilo.visibility === "hidden" ||
          r.width < 2 ||
          r.height < 2
        )
          return []
        const x = r.x + scrollX - marco.x,
          y = r.y + scrollY - marco.y
        // Un control cortado por el encuadre no sirve para indicar dónde pulsar.
        if (
          x < -1 ||
          y < -1 ||
          x + r.width > marco.width + 1 ||
          y + r.height > marco.height + 1
        )
          return []
        const referencias = el
          .getAttribute("aria-labelledby")
          ?.split(/\s+/)
          .map((id) => document.getElementById(id)?.textContent)
          .join(" ")
        const etiquetas = el.labels
          ? [...el.labels].map((l) => l.textContent).join(" ")
          : ""
        const texto = normalizar(
          el.getAttribute("aria-label") ||
            referencias ||
            etiquetas ||
            el.innerText ||
            el.getAttribute("title") ||
            el.getAttribute("placeholder")
        )
        if (!texto) return []
        const tag = el.tagName.toLowerCase()
        const tipo =
          el.getAttribute("role") ||
          (tag === "input"
            ? el.type === "checkbox" || el.type === "radio"
              ? el.type
              : "campo"
            : { select: "campo", textarea: "campo", a: "enlace", summary: "desplegable" }[
                tag
              ] || tag)
        return [
          {
            tipo,
            texto,
            x: Math.max(0, x),
            y: Math.max(0, y),
            ancho: r.width,
            alto: r.height,
          },
        ]
      })
    }, encuadre)
}
