"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { useState, type FormEvent, type ReactNode } from "react";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Input } from "@/components/form";
import { Modal } from "@/components/Modal";
import { LoadingState, PageHeader, Panel } from "@/components/panels";
import { StatusDot } from "@/components/StatusDot";
import { api, fieldErrorOf, type CheckReport, type Server } from "@/lib/api";
import { formatDateTime } from "@/lib/format";
import { CheckReportView } from "@/features/servers/CheckReportView";
import { ContainersPanel } from "@/features/servers/ContainersPanel";
import { formatResources } from "@/features/servers/ServersScreen";

/** Single-server view: facts, checks, containers, and lifecycle actions. */
export function ServerDetailScreen({ serverId }: { serverId: string }) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [report, setReport] = useState<CheckReport>();

  const server = useQuery({
    queryKey: ["server", serverId],
    queryFn: () => api.servers.get(serverId),
  });

  const checks = useMutation({
    mutationFn: () => api.servers.runChecks(serverId),
    onSuccess: async (result) => {
      setReport(result);
      await queryClient.invalidateQueries({ queryKey: ["server", serverId] });
      await queryClient.invalidateQueries({ queryKey: ["servers"] });
    },
  });

  const remove = useMutation({
    mutationFn: () => api.servers.remove(serverId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["servers"] });
      router.push("/servers");
    },
  });

  if (server.isPending) {
    return (
      <main className="mx-auto max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
        <LoadingState label="Loading server…" />
      </main>
    );
  }
  if (server.error || !server.data) {
    return (
      <main className="mx-auto max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
        <ErrorNotice error={server.error ?? new Error("Server not found")} />
      </main>
    );
  }
  const data = server.data;

  return (
    <main className="mx-auto min-h-screen max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
      <PageHeader
        actions={
          <>
            <Button disabled={checks.isPending} onClick={() => checks.mutate()}>
              {checks.isPending ? "Running checks…" : "Re-run checks"}
            </Button>
            <Button onClick={() => setEditing(true)}>Edit</Button>
            <Button onClick={() => setDeleting(true)} variant="danger">
              Delete
            </Button>
          </>
        }
        description={`${data.hostname || data.ipAddress || "no address"} · ${data.sshUser}@${data.sshPort}`}
        eyebrow="Server"
        title={data.name}
      />

      <div className="mt-8 grid gap-6">
        <Panel title="Details">
          <dl className="grid gap-x-8 gap-y-4 text-sm sm:grid-cols-2 lg:grid-cols-3">
            <Fact label="Status">
              <StatusDot status={data.status} />
            </Fact>
            <Fact label="Resources">{formatResources(data.resources)}</Fact>
            <Fact label="OS / architecture">
              {[data.os, data.architecture].filter(Boolean).join(" / ") || "—"}
            </Fact>
            <Fact label="Hostname">{data.hostname || "—"}</Fact>
            <Fact label="IP address">{data.ipAddress || "—"}</Fact>
            <Fact label="Host key">
              {data.hostKeySaved ? "Saved" : "Not saved yet"}
            </Fact>
            <Fact label="Registered">{formatDateTime(data.createdAt)}</Fact>
            <Fact label="Updated">{formatDateTime(data.updatedAt)}</Fact>
          </dl>
        </Panel>

        {checks.error ? <ErrorNotice error={checks.error} /> : null}
        {report ? (
          <Panel title="Check report">
            <CheckReportView report={report} />
          </Panel>
        ) : null}

        <ContainersPanel serverId={serverId} />
      </div>

      {editing ? (
        <EditServerDialog onClose={() => setEditing(false)} server={data} />
      ) : null}
      {deleting ? (
        <ConfirmDialog
          busy={remove.isPending}
          confirmLabel="Delete server"
          danger
          error={remove.error}
          message={`Removes ${data.name} from Ship. Anything still placed on it (services, databases) must be moved first.`}
          onCancel={() => setDeleting(false)}
          onConfirm={() => remove.mutate()}
          title={`Delete ${data.name}?`}
        />
      ) : null}
    </main>
  );
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt className="text-zinc-500">{label}</dt>
      <dd className="mt-1 text-zinc-200">{children}</dd>
    </div>
  );
}

function EditServerDialog({
  server,
  onClose,
}: {
  server: Server;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState(server.name);
  const [hostname, setHostname] = useState(server.hostname ?? "");
  const [ipAddress, setIpAddress] = useState(server.ipAddress ?? "");
  const [sshUser, setSshUser] = useState(server.sshUser);
  const [sshPort, setSshPort] = useState(String(server.sshPort));

  const save = useMutation({
    mutationFn: () =>
      api.servers.update(server.id, {
        name,
        hostname: hostname || undefined,
        ipAddress: ipAddress || undefined,
        sshUser,
        sshPort: Number(sshPort),
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["server", server.id] });
      await queryClient.invalidateQueries({ queryKey: ["servers"] });
      onClose();
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    save.mutate();
  }

  return (
    <Modal onClose={onClose} title={`Edit ${server.name}`}>
      <form className="grid gap-4 sm:grid-cols-2" onSubmit={submit}>
        <Field error={fieldErrorOf(save.error, "name")} label="Name">
          <Input
            onChange={(event) => setName(event.target.value)}
            required
            value={name}
          />
        </Field>
        <Field error={fieldErrorOf(save.error, "hostname")} label="Hostname">
          <Input
            onChange={(event) => setHostname(event.target.value)}
            value={hostname}
          />
        </Field>
        <Field error={fieldErrorOf(save.error, "ipAddress")} label="IP address">
          <Input
            onChange={(event) => setIpAddress(event.target.value)}
            value={ipAddress}
          />
        </Field>
        <div className="grid grid-cols-2 gap-4">
          <Field error={fieldErrorOf(save.error, "sshUser")} label="SSH user">
            <Input
              onChange={(event) => setSshUser(event.target.value)}
              required
              value={sshUser}
            />
          </Field>
          <Field error={fieldErrorOf(save.error, "sshPort")} label="SSH port">
            <Input
              min={1}
              onChange={(event) => setSshPort(event.target.value)}
              required
              type="number"
              value={sshPort}
            />
          </Field>
        </div>
        {save.error ? (
          <div className="sm:col-span-2">
            <ErrorNotice error={save.error} />
          </div>
        ) : null}
        <div className="flex justify-end gap-2 sm:col-span-2">
          <Button onClick={onClose} variant="secondary">
            Cancel
          </Button>
          <Button disabled={save.isPending} type="submit" variant="primary">
            {save.isPending ? "Saving…" : "Save changes"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
