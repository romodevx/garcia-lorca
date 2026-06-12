import type { ButtonHTMLAttributes, ReactNode } from "react";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "primary" | "secondary";
  children: ReactNode;
}

/**
 * Minimal accessible button primitive. A real design-system pass (tokens,
 * sizes, loading state) lands in the design phase — this is the FASE 0 stub.
 */
export function Button({ variant = "primary", children, ...rest }: ButtonProps): ReactNode {
  return (
    <button type="button" data-variant={variant} {...rest}>
      {children}
    </button>
  );
}
