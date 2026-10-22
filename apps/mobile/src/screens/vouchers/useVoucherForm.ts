import { useRef, useState } from "react";
import { sendPaymentVoucher, type FamilyPaymentVoucherResult, type VoucherFile } from "../../api";
import { pickVoucherFile, type VoucherSource } from "../../files/pickVoucher";
import { dayInputOf, parseAmountCents, parseDayInput, sendErrorMessage } from "./voucherInput";

const MESSAGES = {
  fileMissing: "Elige la foto o el PDF del comprobante",
  fileTooLarge: "El archivo pesa más de 5 MB. Elige una foto más ligera o un PDF más pequeño.",
  cameraDenied: "Permite el acceso a la cámara en Ajustes para tomar la foto.",
  fileUnsupported: "Ese formato no se puede enviar. Usa una foto JPG o PNG, o un PDF.",
  amount: "Escribe el importe en pesos, por ejemplo 2,500.00",
  dayInvalid: "Escribe la fecha como 21/10/2026",
  dayFuture: "La fecha no puede ser después de hoy",
};

const PICK_REFUSALS: Record<"denied" | "too_large" | "unsupported", string> = {
  denied: MESSAGES.cameraDenied,
  too_large: MESSAGES.fileTooLarge,
  unsupported: MESSAGES.fileUnsupported,
};

export interface FieldErrors {
  file: string | null;
  amount: string | null;
  day: string | null;
}

const NO_ERRORS: FieldErrors = { file: null, amount: null, day: null };

function amountError(text: string): string | null {
  return parseAmountCents(text) === "invalid" ? MESSAGES.amount : null;
}

function dayError(text: string, today: string): string | null {
  const day = parseDayInput(text, today);
  if (day === "invalid") return MESSAGES.dayInvalid;
  return day === "future" ? MESSAGES.dayFuture : null;
}

/**
 * The voucher form's state and rules (~/.claude/rules/ui-forms-and-modals.md):
 * a missing file is said on submit only, a bad amount or day on blur, and an
 * error shown is re-checked on every keystroke. The submit is never disabled —
 * a second tap while sending is ignored instead.
 */
export function useVoucherForm(studentId: string | null, today: string, onSent: () => void) {
  const [file, setFile] = useState<VoucherFile | null>(null);
  const [amountText, setAmountText] = useState("");
  const [dayText, setDayText] = useState(() => dayInputOf(today));
  const [errors, setErrors] = useState<FieldErrors>(NO_ERRORS);
  const [showSummary, setShowSummary] = useState(false);
  const [sendError, setSendError] = useState<string | null>(null);
  const [sent, setSent] = useState<FamilyPaymentVoucherResult | null>(null);
  const [busy, setBusy] = useState(false);
  const sending = useRef(false);
  // What was last typed, for a blur that lands before the re-render does.
  const typed = useRef({ amount: "", day: dayText });

  const setError = (field: keyof FieldErrors, message: string | null): void => setErrors((e) => ({ ...e, [field]: message }));

  const pick = async (source: VoucherSource): Promise<void> => {
    const picked = await pickVoucherFile(source);
    if (picked.kind === "canceled") return;
    if (picked.kind === "picked") {
      setFile(picked.file);
      setError("file", null);
      return;
    }
    setError("file", PICK_REFUSALS[picked.kind]);
  };

  const changeAmount = (text: string): void => {
    setAmountText(text);
    typed.current.amount = text;
    if (errors.amount !== null) setError("amount", amountError(text));
  };
  const changeDay = (text: string): void => {
    setDayText(text);
    typed.current.day = text;
    if (errors.day !== null) setError("day", dayError(text, today));
  };

  const submit = async (): Promise<void> => {
    if (sending.current || studentId === null) return;
    const checked: FieldErrors = { file: file === null ? (errors.file ?? MESSAGES.fileMissing) : null, amount: amountError(amountText), day: dayError(dayText, today) };
    setErrors(checked);
    setSendError(null);
    setSent(null);
    if (file === null || checked.amount !== null || checked.day !== null) {
      setShowSummary(true);
      return;
    }
    setShowSummary(false);
    sending.current = true;
    setBusy(true);
    try {
      const amountCents = parseAmountCents(amountText);
      const paidOn = parseDayInput(dayText, today);
      const result = await sendPaymentVoucher(studentId, {
        file,
        ...(typeof amountCents === "number" ? { amountCents } : {}),
        ...(paidOn !== null ? { paidOn } : {}),
      });
      setSent(result);
      setFile(null);
      setAmountText("");
      setDayText(dayInputOf(today));
      typed.current = { amount: "", day: dayInputOf(today) };
      onSent();
    } catch (error) {
      setSendError(sendErrorMessage(error));
    } finally {
      sending.current = false;
      setBusy(false);
    }
  };

  const summary = showSummary ? [errors.file, errors.amount, errors.day].filter((e): e is string => e !== null) : [];

  return {
    file,
    amountText,
    dayText,
    errors,
    summary,
    sendError,
    sent,
    busy,
    pick,
    changeAmount,
    changeDay,
    blurAmount: () => setError("amount", amountError(typed.current.amount)),
    blurDay: () => setError("day", dayError(typed.current.day, today)),
    submit,
  };
}

export type VoucherFormState = ReturnType<typeof useVoucherForm>;
