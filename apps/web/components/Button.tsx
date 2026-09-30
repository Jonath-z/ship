import type { ButtonHTMLAttributes } from "react";

type Variant = "primary" | "secondary" | "danger" | "ghost";
type Size = "sm" | "md";

const variants: Record<Variant, string> = {
  primary:
    "bg-zinc-50 font-medium text-zinc-950 hover:bg-zinc-300 disabled:hover:bg-zinc-50",
  secondary:
    "border border-zinc-700 bg-transparent font-medium text-zinc-100 hover:border-zinc-500 hover:bg-zinc-900 disabled:hover:border-zinc-700 disabled:hover:bg-transparent",
  danger:
    "border border-red-900/80 font-medium text-red-400 hover:border-red-700 hover:bg-red-950/40 disabled:hover:border-red-900/80 disabled:hover:bg-transparent",
  ghost: "font-medium text-zinc-400 hover:bg-zinc-900 hover:text-zinc-100",
};

const sizes: Record<Size, string> = {
  sm: "h-8 px-3 text-sm",
  md: "h-9 px-4 text-sm",
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
      className={`inline-flex items-center justify-center gap-2 rounded-md transition-colors duration-150 disabled:cursor-not-allowed disabled:opacity-50 ${variants[variant]} ${sizes[size]} ${className}`}
      type={type}
      {...props}
    />
  );
}
