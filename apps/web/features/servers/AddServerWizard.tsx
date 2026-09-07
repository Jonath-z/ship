"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Input, Select, Textarea } from "@/components/form";
import { Modal } from "@/components/Modal";
import {
  api,
  fieldErrorOf,
  type CheckReport,
  type PrepareReport,
  type Server,
  type SshKey,
} from "@/lib/api";
import { useSshKeys } from "@/lib/hooks";
import { CheckReportView } from "@/features/servers/CheckReportView";
import { CopyButton } from "@/features/servers/SshKeysPanel";

type Step = "key" | "register" | "checks" | "prepare";

const stepLabels: Record<Step, string> = {
  key: "1 · SSH key",
  register: "2 · Register",
  checks: "3 · Checks",
  prepare: "4 · Prepare",
};

/** Multi-step flow: pick/create a key, register the host, verify, prepare. */
export function AddServerWizard({ onClose }: { onClose: () => void }) {
  const [step, setStep] = useState<Step>("key");
  const [sshKeyId, setSshKeyId] = useState<string>();
  const [server, setServer] = useState<Server>();

  return (
    <Modal onClose={onClose} title="Add server" wide>
      <p className="mb-4 text-xs font-medium uppercase tracking-wide text-emerald-400">
        {stepLabels[step]}
      </p>
      {step === "key" ? (
        <KeyStep
          onNext={(keyId) => {
            setSshKeyId(keyId);
            setStep("register");
          }}
        />
      ) : null}
      {step === "register" && sshKeyId ? (
        <RegisterStep
          onNext={(created) => {
            setServer(created);
            setStep("checks");
          }}
          sshKeyId={sshKeyId}
        />
      ) : null}
      {step === "checks" && server ? (
        <ChecksStep onNext={() => setStep("prepare")} server={server} />
      ) : null}
      {step === "prepare" && server ? (
        <PrepareStep onDone={onClose} server={server} />
      ) : null}
    </Modal>
  );
}

function KeyStep({ onNext }: { onNext: (sshKeyId: string) => void }) {
  const queryClient = useQueryClient();
  const { data, isPending, error } = useSshKeys();
  const keys = data?.items ?? [];
  const [mode, setMode] = useState<"existing" | "generate" | "import">(
    "existing",
  );
  const [selected, setSelected] = useState("");
  const [name, setName] = useState("");
  const [privateKey, setPrivateKey] = useState("");
  const [generated, setGenerated] = useState<SshKey>();

  const create = useMutation({
    mutationFn: () =>
      api.sshKeys.create(mode === "import" ? { name, privateKey } : { name }),
    onSuccess: async (key) => {
      await queryClient.invalidateQueries({ queryKey: ["ssh-keys"] });
      if (mode === "generate") setGenerated(key);
      else onNext(key.id);
    },
  });

  if (generated) {
    return (
      <div>
        <p className="text-sm text-zinc-400">
          Add this public key to <code>~/.ssh/authorized_keys</code> on the
          server, then continue.
        </p>
        <pre className="mt-3 overflow-x-auto whitespace-pre-wrap break-all rounded-lg border border-zinc-800 bg-zinc-950 p-3 font-mono text-xs text-zinc-300">
          {generated.publicKey}
        </pre>
        <div className="mt-5 flex justify-end gap-2">
          <CopyButton label="Copy public key" text={generated.publicKey} />
          <Button onClick={() => onNext(generated.id)} variant="primary">
            Continue
          </Button>
        </div>
      </div>
    );
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (mode === "existing") onNext(selected);
    else create.mutate();
  }

  return (
    <form className="grid gap-4" onSubmit={submit}>
      {error ? <ErrorNotice error={error} /> : null}
      <div className="flex gap-4 text-sm text-zinc-300">
        {(
          [
            ["existing", "Use existing key"],
            ["generate", "Generate new"],
            ["import", "Import private key"],
          ] as const
        ).map(([value, label]) => (
          <label className="flex items-center gap-2" key={value}>
            <input
              checked={mode === value}
              className="accent-emerald-400"
              disabled={value === "existing" && keys.length === 0}
              onChange={() => setMode(value)}
              type="radio"
            />
            {label}
          </label>
        ))}
      </div>
      {mode === "existing" ? (
        <Field label="SSH key">
          <Select
            disabled={isPending || keys.length === 0}
            onChange={(event) => setSelected(event.target.value)}
            required
            value={selected}
          >
            <option disabled value="">
              {keys.length === 0 ? "No keys available" : "Choose a key…"}
            </option>
            {keys.map((key) => (
              <option key={key.id} value={key.id}>
                {key.name}
              </option>
            ))}
          </Select>
        </Field>
      ) : (
        <Field error={fieldErrorOf(create.error, "name")} label="Key name">
          <Input
            onChange={(event) => setName(event.target.value)}
            placeholder="deploy-key"
            required
            value={name}
          />
        </Field>
      )}
      {mode === "import" ? (
        <Field
          error={fieldErrorOf(create.error, "privateKey")}
          label="Private key"
        >
          <Textarea
            className="font-mono"
            onChange={(event) => setPrivateKey(event.target.value)}
            placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
            required
            rows={5}
            value={privateKey}
          />
        </Field>
      ) : null}
      {create.error ? <ErrorNotice error={create.error} /> : null}
      <div className="flex justify-end">
        <Button
          disabled={create.isPending || (mode === "existing" && !selected)}
          type="submit"
          variant="primary"
        >
          {create.isPending ? "Working…" : "Continue"}
        </Button>
      </div>
    </form>
  );
}

function RegisterStep({
  sshKeyId,
  onNext,
}: {
  sshKeyId: string;
  onNext: (server: Server) => void;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [hostname, setHostname] = useState("");
  const [ipAddress, setIpAddress] = useState("");
  const [sshUser, setSshUser] = useState("root");
  const [sshPort, setSshPort] = useState("22");

  const register = useMutation({
    mutationFn: () =>
      api.servers.create({
        name,
        hostname: hostname || undefined,
        ipAddress: ipAddress || undefined,
        sshUser,
        sshPort: Number(sshPort),
        sshKeyId,
      }),
    onSuccess: async (server) => {
      await queryClient.invalidateQueries({ queryKey: ["servers"] });
      onNext(server);
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    register.mutate();
  }

  return (
    <form className="grid gap-4 sm:grid-cols-2" onSubmit={submit}>
      <Field error={fieldErrorOf(register.error, "name")} label="Name">
        <Input
          autoFocus
          onChange={(event) => setName(event.target.value)}
          placeholder="web-1"
          required
          value={name}
        />
      </Field>
      <Field
        error={fieldErrorOf(register.error, "hostname")}
        label="Hostname"
      >
        <Input
          onChange={(event) => setHostname(event.target.value)}
          placeholder="web-1.example.com"
          value={hostname}
        />
      </Field>
      <Field
        error={fieldErrorOf(register.error, "ipAddress")}
        hint="Hostname or IP address is required."
        label="IP address"
      >
        <Input
          onChange={(event) => setIpAddress(event.target.value)}
          placeholder="203.0.113.10"
          value={ipAddress}
        />
      </Field>
      <div className="grid grid-cols-2 gap-4">
        <Field error={fieldErrorOf(register.error, "sshUser")} label="SSH user">
          <Input
            onChange={(event) => setSshUser(event.target.value)}
            required
            value={sshUser}
          />
        </Field>
        <Field error={fieldErrorOf(register.error, "sshPort")} label="SSH port">
          <Input
            min={1}
            onChange={(event) => setSshPort(event.target.value)}
            required
            type="number"
            value={sshPort}
          />
        </Field>
      </div>
      {register.error ? (
        <div className="sm:col-span-2">
          <ErrorNotice error={register.error} />
        </div>
      ) : null}
      <div className="flex justify-end sm:col-span-2">
        <Button disabled={register.isPending} type="submit" variant="primary">
          {register.isPending ? "Registering…" : "Register server"}
        </Button>
      </div>
    </form>
  );
}

function ChecksStep({
  server,
  onNext,
}: {
  server: Server;
  onNext: () => void;
}) {
  const queryClient = useQueryClient();
  const [report, setReport] = useState<CheckReport>();

  const checks = useMutation({
    mutationFn: () => api.servers.runChecks(server.id),
    onSuccess: async (result) => {
      setReport(result);
      await queryClient.invalidateQueries({ queryKey: ["servers"] });
    },
  });

  return (
    <div className="grid gap-4">
      <p className="text-sm text-zinc-400">
        Verify SSH connectivity and requirements on{" "}
        <span className="text-zinc-200">{server.name}</span>.
      </p>
      {checks.error ? <ErrorNotice error={checks.error} /> : null}
      {report ? <CheckReportView report={report} /> : null}
      <div className="flex justify-end gap-2">
        <Button disabled={checks.isPending} onClick={() => checks.mutate()}>
          {checks.isPending
            ? "Running checks…"
            : report
              ? "Re-run checks"
              : "Run checks"}
        </Button>
        <Button disabled={!report} onClick={onNext} variant="primary">
          Continue
        </Button>
      </div>
    </div>
  );
}

function PrepareStep({
  server,
  onDone,
}: {
  server: Server;
  onDone: () => void;
}) {
  const queryClient = useQueryClient();
  const [result, setResult] = useState<PrepareReport>();

  const prepare = useMutation({
    mutationFn: () => api.servers.prepare(server.id),
    onSuccess: async (report) => {
      setResult(report);
      await queryClient.invalidateQueries({ queryKey: ["servers"] });
    },
  });

  return (
    <div className="grid gap-4">
      <p className="text-sm text-zinc-400">
        Optionally install Docker and configure{" "}
        <span className="text-zinc-200">{server.name}</span> for deployments.
        This can take a few minutes.
      </p>
      {prepare.isPending ? (
        <p className="flex items-center gap-2 text-sm text-emerald-300">
          <span className="inline-block h-3 w-3 animate-spin rounded-full border-2 border-emerald-400 border-t-transparent" />
          Preparing server — keep this window open…
        </p>
      ) : null}
      {prepare.error ? <ErrorNotice error={prepare.error} /> : null}
      {result ? (
        <>
          <pre className="max-h-64 overflow-y-auto rounded-lg border border-zinc-800 bg-zinc-950 p-3 font-mono text-xs text-zinc-300">
            {result.log.join("\n") || "No output."}
          </pre>
          <CheckReportView report={result.report} />
        </>
      ) : null}
      <div className="flex justify-end gap-2">
        {!result ? (
          <Button disabled={prepare.isPending} onClick={onDone} variant="ghost">
            Skip
          </Button>
        ) : null}
        {!result ? (
          <Button
            disabled={prepare.isPending}
            onClick={() => prepare.mutate()}
            variant="primary"
          >
            {prepare.isPending ? "Preparing…" : "Prepare server"}
          </Button>
        ) : (
          <Button onClick={onDone} variant="primary">
            Done
          </Button>
        )}
      </div>
    </div>
  );
}
