import * as broker from "./gen/broker/protocol/broker.ts";
import { Result } from "./result.ts";

export type JsonObject = Record<string, unknown>;

export function trimUrl(orig: URL): string {
  let url = orig.toString();
  if (url.endsWith("/")) {
    url = url.slice(0, url.length - 1);
  }
  return url;
}

/// A request that never produced an HTTP response. `fetch` rejects when the
/// failure happens below HTTP: DNS lookup, TLS handshake, a refused or reset
/// connection, a Content-Security-Policy block, or an abort. Runtimes report
/// these as a bare `TypeError`, so there is no status and no body to read.
export class NetworkFailure {
  readonly cause: unknown;
  readonly url: string;

  constructor(url: string, cause: unknown) {
    this.cause = cause;
    this.url = url;
  }
}

export async function doFetch(
    url: string,
    authToken: string,
    body: unknown,
  ): Promise<Result<Response, Response | NetworkFailure>> {
    try {
      const response = await fetch(url, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Authorization": `Bearer ${authToken}`,
        },
        body: JSON.stringify(body),
      });

      if (response.ok) {
        return Result.Ok(response);
      } else {
        return Result.Err(response);
      }
    } catch (cause) {
      return Result.Err(new NetworkFailure(url, cause));
    }
  }

function describeCause(cause: unknown): string {
  if (cause instanceof Error) {
    return cause.message;
  }

  if (typeof cause === "string") {
    return cause;
  }

  return String(cause);
}

/// Reads the body of an error response. A proxy or load balancer in front of
/// the gateway can answer with HTML or plain text, and a connection can drop
/// part way through the body, so neither the read nor the parse is certain to
/// succeed. Both failures return the status line instead of throwing, because
/// a throw here replaces the error the caller is trying to report.
// deno-lint-ignore no-explicit-any
async function readErrorBody(response: Response): Promise<any> {
  let text: string;

  try {
    text = await response.text();
  } catch (cause) {
    return { code: response.status, message: describeCause(cause) };
  }

  try {
    return JSON.parse(text);
  } catch {
    return { code: response.status, message: text.trim() || response.statusText };
  }
}

/// Transforms Unary and Stream errors into a similar structure.
export class ResponseError {
  readonly body: broker.RuntimeError | broker.RuntimeStreamError;
  /// Absent when the request failed before a response arrived.
  readonly response?: Response;
  readonly status: string;

  private constructor(
    body: broker.RuntimeError | broker.RuntimeStreamError,
    response: Response | undefined,
    status: string,
  ) {
    this.body = body;
    this.response = response;
    this.status = status;
  }

  static readonly NETWORK_FAILURE_STATUS = "NetworkFailure";

  private static fromNetworkFailure(failure: NetworkFailure): ResponseError {
    return new ResponseError(
      {
        code: 0,
        message: `Could not reach ${failure.url}: ${
          describeCause(failure.cause)
        }`,
      },
      undefined,
      ResponseError.NETWORK_FAILURE_STATUS,
    );
  }

  static async fromResponse(
    source: Response | NetworkFailure,
  ): Promise<ResponseError> {
    if (source instanceof NetworkFailure) {
      return ResponseError.fromNetworkFailure(source);
    }

    const body = await readErrorBody(source);
    const status = body?.code;

    return new ResponseError(body, source, status);
  }

  static async fromStreamResponse(
    source: Response | NetworkFailure,
  ): Promise<ResponseError> {
    if (source instanceof NetworkFailure) {
      return ResponseError.fromNetworkFailure(source);
    }

    const body = await readErrorBody(source);
    // grpc-gateway wraps a streaming RPC's error in an `error` envelope. A
    // proxy that answers ahead of the gateway does not, so fall back to the
    // whole body.
    const error = body?.error ?? body;
    const status = error?.httpStatus ?? `${source.status}`;

    return new ResponseError(error, source, status);
  }
}

export type sortFn<T> = (a: T, b: T) => number;

export function sortBy<T, K extends keyof T>(...fields: Array<K>): sortFn<T> {
  return (a, b) => {

    for (let i = 0; i < fields.length; i++) {
      const field = fields[i];
      if (a[field] > b[field]) {
        return 1;
      } else if (a[field] < b[field]) {
        return -1;
      } else {
        // a and b have the same field values. Try the next field.
      }
    }

    // a and b are equal across all fields
    return 0;
  };
}
