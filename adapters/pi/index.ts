type HitchMutation = {
  path: string[];
  value: unknown;
};

type HitchAdapterResponse = {
  adapter_action: "noop" | "return" | "mutate_and_return";
  return_value?: unknown;
  mutations?: HitchMutation[];
};

type HookEvent = {
  input?: unknown;
  [key: string]: unknown;
};

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function applyMutation(target: HookEvent, mutation: HitchMutation): void {
  if (mutation.path.length === 0) return;

  let cursor: unknown = target;
  for (let i = 0; i < mutation.path.length - 1; i += 1) {
    if (!isObject(cursor)) return;
    cursor = cursor[mutation.path[i]];
  }

  if (!isObject(cursor)) return;
  cursor[mutation.path[mutation.path.length - 1]] = mutation.value;
}

export function applyHitchResponse(event: HookEvent, response: HitchAdapterResponse): unknown {
  if (process.env.HITCH_CHILD === "1") return undefined;
  if (response.adapter_action === "mutate_and_return") {
    for (const mutation of response.mutations ?? []) applyMutation(event, mutation);
    return response.return_value;
  }
  if (response.adapter_action === "return") return response.return_value;
  return undefined;
}

export async function postToHitch(baseUrl: string, body: unknown, sync: boolean): Promise<HitchAdapterResponse | undefined> {
  const path = sync ? "/v1/dispatch-sync" : "/v1/events";
  const res = await fetch(`${baseUrl}${path}`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) return undefined;
  const payload: unknown = await res.json();
  if (!sync || !isObject(payload)) return undefined;
  const nativeResponse = payload.native_response;
  return isObject(nativeResponse) && typeof nativeResponse.adapter_action === "string"
    ? nativeResponse as HitchAdapterResponse
    : undefined;
}
