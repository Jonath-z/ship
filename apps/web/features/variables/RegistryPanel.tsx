"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Input } from "@/components/form";
import { Panel } from "@/components/panels";
import { api } from "@/lib/api";

// Kamal's conventional names; the renderer lifts these into the registry
// block of every deploy.yml and keeps them out of application env.
const SERVER_VAR = "KAMAL_REGISTRY_SERVER";
const USERNAME_SECRET = "KAMAL_REGISTRY_USERNAME";
const PASSWORD_SECRET = "KAMAL_REGISTRY_PASSWORD";

/**
 * Container registry credentials for this environment. Stored as one clear
 * variable (server) and two encrypted secrets (username, password); Kamal
 * logs in on the target servers with them before pulling images.
 */
export function RegistryPanel({
  projectId,
  environmentId,
}: {
  projectId: string;
  environmentId: string;
}) {
  const queryClient = useQueryClient();
  const [server, setServer] = useState<string>();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");

  const variables = useQuery({
    queryKey: ["variables", projectId, environmentId],
    queryFn: () => api.variables.list(projectId, environmentId),
  });
  const secrets = useQuery({
    queryKey: ["secrets", projectId, environmentId],
    queryFn: () => api.secrets.list(projectId, environmentId),
  });

  const serverVariable = variables.data?.items.find(
    (variable) => variable.name === SERVER_VAR && !variable.serviceId,
  );
  const usernameSecret = secrets.data?.items.find(
    (secret) => secret.name === USERNAME_SECRET && !secret.serviceId,
  );
  const passwordSecret = secrets.data?.items.find(
    (secret) => secret.name === PASSWORD_SECRET && !secret.serviceId,
  );
  const configured = Boolean(passwordSecret);

  const save = useMutation({
    mutationFn: async () => {
      const serverValue = server ?? serverVariable?.value ?? "";
      if (serverValue && serverVariable) {
        if (serverValue !== serverVariable.value) {
          await api.variables.update(projectId, environmentId, serverVariable.id, {
            value: serverValue,
          });
        }
      } else if (serverValue) {
        await api.variables.create(projectId, environmentId, {
          name: SERVER_VAR,
          value: serverValue,
        });
      } else if (serverVariable) {
        await api.variables.remove(projectId, environmentId, serverVariable.id);
      }

      if (username) {
        if (usernameSecret) {
          await api.secrets.update(projectId, environmentId, usernameSecret.id, {
            value: username,
          });
        } else {
          await api.secrets.create(projectId, environmentId, {
            name: USERNAME_SECRET,
            value: username,
          });
        }
      }
      if (password) {
        if (passwordSecret) {
          await api.secrets.update(projectId, environmentId, passwordSecret.id, {
            value: password,
          });
        } else {
          await api.secrets.create(projectId, environmentId, {
            name: PASSWORD_SECRET,
            value: password,
          });
        }
      }
    },
    onSuccess: async () => {
      setServer(undefined);
      setUsername("");
      setPassword("");
      await queryClient.invalidateQueries({
        queryKey: ["variables", projectId, environmentId],
      });
      await queryClient.invalidateQueries({
        queryKey: ["secrets", projectId, environmentId],
      });
      await queryClient.invalidateQueries({
        queryKey: ["configuration-preview", projectId, environmentId],
      });
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    save.mutate();
  }

  return (
    <Panel
      actions={
        configured ? (
          <Badge tone="emerald">configured</Badge>
        ) : (
          <Badge tone="amber">not configured</Badge>
        )
      }
      description="Kamal logs in to your image registry on the target servers before pulling. Use a read-only token (e.g. a GitHub PAT with read:packages) rather than your account password."
      title="Registry credentials"
    >
      <form className="grid gap-4 sm:grid-cols-3" onSubmit={submit}>
        <Field
          hint="Empty means Docker Hub."
          label="Registry server"
        >
          <Input
            onChange={(event) => setServer(event.target.value)}
            placeholder="ghcr.io"
            value={server ?? serverVariable?.value ?? ""}
          />
        </Field>
        <Field
          hint={usernameSecret ? "Set — enter a value to replace it." : undefined}
          label="Username"
        >
          <Input
            autoComplete="off"
            onChange={(event) => setUsername(event.target.value)}
            placeholder={usernameSecret ? "••••••••" : "registry user"}
            value={username}
          />
        </Field>
        <Field
          hint={passwordSecret ? "Set — enter a value to replace it." : undefined}
          label="Password / token"
        >
          <Input
            autoComplete="new-password"
            onChange={(event) => setPassword(event.target.value)}
            placeholder={passwordSecret ? "••••••••" : "access token"}
            type="password"
            value={password}
          />
        </Field>
        {save.error ? (
          <div className="sm:col-span-3">
            <ErrorNotice error={save.error} />
          </div>
        ) : null}
        <div className="flex justify-end sm:col-span-3">
          <Button
            disabled={
              save.isPending ||
              (server === undefined && !username && !password)
            }
            type="submit"
            variant="primary"
          >
            {save.isPending ? "Saving…" : "Save credentials"}
          </Button>
        </div>
      </form>
    </Panel>
  );
}
