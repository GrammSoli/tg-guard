import { useMemo } from "react";

/**
 * Wraps date text with a native tooltip showing the user's timezone.
 * Use instead of bare date strings to hint that dates are local.
 */
export function DateTz({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  const tz = useMemo(() => {
    try {
      return Intl.DateTimeFormat().resolvedOptions().timeZone;
    } catch {
      return "";
    }
  }, []);

  return (
    <span className={className} title={tz ? `🕐 ${tz}` : undefined}>
      {children}
    </span>
  );
}
