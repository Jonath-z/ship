import { ApiError } from "@/lib/api";

/**
 * Renders an API error envelope: the top-level message plus any field-level
 * validation details ({error: {code, message, requestId, details}}).
 */
export function ErrorNotice({ error }: { error: unknown }) {
  if (!error) return null;
  const apiError = error instanceof ApiError ? error : undefined;
  const message =
    apiError?.message ??
    (error instanceof Error ? error.message : "Something went wrong.");
  return (
    <div className="rounded-lg border border-red-900 bg-red-950/40 p-3 text-sm text-red-200">
      <p>{message}</p>
      {apiError && apiError.details.length > 0 ? (
        <ul className="mt-2 list-disc space-y-1 pl-5 text-xs text-red-300">
          {apiError.details.map((detail, index) => (
            <li key={`${detail.field}-${index}`}>
              <span className="font-medium">{detail.field}</span>:{" "}
              {detail.message}
            </li>
          ))}
        </ul>
      ) : null}
      {apiError?.requestId ? (
        <p className="mt-2 text-xs text-red-400/70">
          Request ID: {apiError.requestId}
        </p>
      ) : null}
    </div>
  );
}
