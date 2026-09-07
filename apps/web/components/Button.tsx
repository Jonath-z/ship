import type { ButtonHTMLAttributes } from "react";

type Variant = "primary" | "secondary" | "danger" | "ghost";
type Size = "sm" | "md";

const variants: Record<Variant, string> = {
  primary:
    "bg-emerald-400 font-semibold text-zinc-950 hover:bg-emerald-300 disabled:hover:bg-emerald-400",
  secondary:
    "border border-zinc-700 text-zinc-200 hover:bg-zinc-800 disabled:hover:bg-transparent",
  danger:
    "border border-red-900 text-red-300 hover:bg-red-950/60 disabled:hover:bg-transparent",
  ghost: "text-zinc-400 hover:text-zinc-100",
};

const sizes: Record<Size, string> = {
  sm: "px-3 py-1.5 text-sm",
  md: "px-4 py-2 text-sm",
};

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
}

export function Button({
  variant = "secondary",
  size = "md",
  className = "",
  type = "button",
  ...props
}: ButtonProps) {
  return (
    <button
      className={`rounded-lg disabled:cursor-not-allowed disabled:opacity-50 ${variants[variant]} ${sizes[size]} ${className}`}
      type={type}
      {...props}
    />
  );
}
