// The rechvix mark: a rupee glyph on a rounded accent tile. Replaces the
// plain flat-color square previously duplicated across AppShell, LoginPage,
// and BootstrapPage — one real (if simple) mark instead of three copies of
// an empty placeholder swatch. Kept as inline SVG (not a raster asset) so
// it stays crisp at any size and needs no extra network request.
export function Logo({ size = 22 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 22 22"
      aria-hidden="true"
      focusable="false"
    >
      <rect width="22" height="22" rx="5" fill="var(--color-accent)" />
      <text
        x="11"
        y="15.5"
        textAnchor="middle"
        fontFamily="var(--font-display)"
        fontSize="13"
        fontWeight="700"
        fill="var(--color-on-accent)"
      >
        ₹
      </text>
    </svg>
  );
}
