/** "Buenos días" until noon, "Buenas tardes" until 7 pm, then "Buenas noches" (B1). */
export function greetingFor(now: Date): string {
  const hour = now.getHours();
  return hour < 12 ? "Buenos días" : hour < 19 ? "Buenas tardes" : "Buenas noches";
}

/**
 * The date under the greeting, es-MX, as a sentence: "Sábado, 17 de octubre"
 * (B2). Only the first letter goes up — CSS `capitalize` raised every word and
 * wrote "17 De Octubre" (CC-315).
 */
export function todayLabel(now: Date): string {
  const text = now.toLocaleDateString("es-MX", { weekday: "long", day: "numeric", month: "long" });
  return text.charAt(0).toLocaleUpperCase("es-MX") + text.slice(1);
}
