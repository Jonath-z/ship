import { SystemDashboard } from "@/features/system/SystemDashboard";
import { AppHeader } from "@/features/auth/AppHeader";

export default function DashboardPage() {
  return (
    <>
      <AppHeader />
      <SystemDashboard />
    </>
  );
}
