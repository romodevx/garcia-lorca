/** "Buenos días" until noon, "Buenas tardes" until 7 pm, then "Buenas noches" (B1). */
export function greetingFor(now: Date): string {
  const hour = now.getHours();
  return hour < 12 ? "Buenos días" : hour < 19 ? "Buenas tardes" : "Buenas noches";
}

/** The date under the greeting, es-MX, capitalized by CSS (B2). */
export function todayLabel(now: Date): string {
  return now.toLocaleDateString("es-MX", { weekday: "long", day: "numeric", month: "long" });
}
