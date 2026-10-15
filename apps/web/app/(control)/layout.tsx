import type { ReactNode } from "react";
import { AppShell } from "@/features/shell/AppShell";

/**
 * Control-plane level of the dashboard: projects, servers, system status,
 * and account/user settings. The route group keeps the URLs unchanged while
 * sharing one shell; environment-scoped pages live under /env instead.
 */
export default function ControlPlaneLayout({
  children,
}: {
  children: ReactNode;
}) {
  return <AppShell>{children}</AppShell>;
}
