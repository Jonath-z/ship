import type { ReactNode } from "react";
import { AppShell } from "@/features/shell/AppShell";

export default function ServersLayout({ children }: { children: ReactNode }) {
  return <AppShell>{children}</AppShell>;
}
