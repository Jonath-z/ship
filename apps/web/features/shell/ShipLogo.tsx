/** Minimal geometric mark used in the top navigation. */
export function ShipLogo({ size = 22 }: { size?: number }) {
  return (
    <svg
      aria-hidden
      fill="currentColor"
      height={size}
      viewBox="0 0 24 24"
      width={size}
    >
      {/* Hull + sail: a nod to "ship" in the spirit of Vercel's triangle. */}
      <path d="M12 2 L19 14 H5 Z" />
      <path d="M4 17 H20 L17.5 21 H6.5 Z" opacity="0.6" />
    </svg>
  );
}
