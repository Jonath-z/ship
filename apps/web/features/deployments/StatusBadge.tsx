import { Badge, type BadgeTone } from "@/components/Badge";
import type { DeploymentStatus } from "@/lib/api";

const tones: Record<DeploymentStatus, BadgeTone> = {
  QUEUED: "zinc",
  VALIDATING: "sky",
  BUILDING: "sky",
  PUSHING: "sky",
  DEPLOYING: "sky",
  VERIFYING: "sky",
  SUCCESS: "emerald",
  FAILED: "red",
  ROLLING_BACK: "amber",
  ROLLED_BACK: "amber",
};

export function StatusBadge({ status }: { status: DeploymentStatus }) {
  return <Badge tone={tones[status] ?? "zinc"}>{status}</Badge>;
}
