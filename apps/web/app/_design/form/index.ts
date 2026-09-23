/**
 * La capa de captura del sistema.
 *
 * Todo input de toda pantalla entra por aquí. Antes eran ~50 inputs con
 * estilos en línea repartidos en 23 archivos: sólo 3 usaban un calendario, sólo
 * 2 lugares en toda la web decían `required`, y el formateo de dinero estaba
 * triplicado con un redondeo distinto en cada copia.
 *
 * Las reglas que esta capa garantiza, y que ninguna pantalla puede desobedecer
 * por descuido:
 *
 *   - Toda fecha se elige en calendario (DateField).
 *   - Nombres y datos de catálogo se capturan en mayúsculas (TextField uppercase).
 *   - El correo, siempre en minúsculas (EmailField).
 *   - Los montos se ven en pesos y viajan en centavos (MoneyField).
 *   - El cuerpo de un aviso NO se grita (TextArea, sin mayúsculas).
 *   - Lo obligatorio lleva asterisco, y al guardar se marca en rojo con halo
 *     (Field + useRequired).
 *   - NINGÚN campo pinta un valor de ejemplo: `placeholder` no existe en esta
 *     capa. Lo que hay que capturar se explica en el (?) de la etiqueta
 *     (Field `help` + HelpTip).
 */

export { Field, inputStyle, type FieldProps } from "./Field";
export { HelpTip, type HelpTipProps } from "./HelpTip";
export { useRequired, FALTA, type RequiredState } from "./useRequired";
export { useFocusFirstInvalid } from "./useFocusFirstInvalid";
export { useFormatError, type FormatErrorState, type FormatRule } from "./useFormatError";
export {
  EmailField,
  NumberField,
  SelectField,
  TextArea,
  TextField,
  type BaseFieldProps,
  type NumberFieldProps,
  type SelectOption,
  type TextFieldProps,
} from "./fields";
export { DateField, type DateFieldProps } from "./DateField";
export { MoneyField } from "./MoneyField";
