import nacl from "tweetnacl";

import type {
  RemoteCapability,
  RemoteConfigPayload,
  SignedRemoteConfigEnvelope,
} from "../../../packages/contracts/src/mobile-policy.ts";

export type RemoteConfigStorage = {
  load(): Promise<string | null>;
  save(value: string): Promise<void>;
  clear(): Promise<void>;
};

export type TrustedRemoteConfigKeys = Readonly<Record<string, string>>;
export type RemoteConfigSource = "NETWORK" | "LAST_KNOWN_SAFE" | "FAIL_SAFE";

export type ResolvedRemoteConfig = {
  source: RemoteConfigSource;
  envelope?: SignedRemoteConfigEnvelope;
  disabledCapabilities: RemoteCapability[];
  reasonCode: string;
};

type CachedRemoteConfigState = {
  verifiedEnvelope: SignedRemoteConfigEnvelope;
  applicableEnvelope: SignedRemoteConfigEnvelope | null;
};

const remoteCapabilities: RemoteCapability[] = [
  "REALTIME_CONSULTATION",
  "CALENDAR_INTEGRATION",
  "PERSONA_PREVIEW",
];

const signaturePattern = /^[A-Za-z0-9+/]{86}==$/;

export async function resolveRemoteConfig(input: {
  baseURL: string;
  storage: RemoteConfigStorage;
  trustedKeys: TrustedRemoteConfigKeys;
  now?: Date;
}): Promise<ResolvedRemoteConfig> {
  const now = input.now ?? new Date();
  const cachedState = await loadCachedState(input.storage, input.trustedKeys, now);
  const cached = cachedState?.applicableEnvelope ?? null;
  const highWater = cachedState?.verifiedEnvelope ?? null;

  let fetched: SignedRemoteConfigEnvelope;
  try {
    fetched = await fetchRemoteConfig(input.baseURL, input.trustedKeys, now);
  } catch {
    if (cached) {
      return fromEnvelope("LAST_KNOWN_SAFE", cached, "REMOTE_CONFIG_FETCH_FAILED");
    }
    return failSafe("REMOTE_CONFIG_UNAVAILABLE_FAIL_SAFE");
  }

  if (highWater && fetched.payload.version < highWater.payload.version) {
    return cached
      ? fromEnvelope("LAST_KNOWN_SAFE", cached, "REMOTE_CONFIG_ROLLBACK_REJECTED")
      : failSafe("REMOTE_CONFIG_ROLLBACK_REJECTED");
  }
  if (
    highWater &&
    fetched.payload.version === highWater.payload.version &&
    !sameSemanticConfig(fetched, highWater)
  ) {
    return cached
      ? fromEnvelope("LAST_KNOWN_SAFE", cached, "REMOTE_CONFIG_SAME_VERSION_CHANGED")
      : failSafe("REMOTE_CONFIG_SAME_VERSION_CHANGED");
  }

  try {
    await input.storage.save(JSON.stringify(fetched));
  } catch {
    return fromEnvelope("NETWORK", fetched, "REMOTE_CONFIG_APPLIED_PERSISTENCE_FAILED");
  }
  return fromEnvelope("NETWORK", fetched, "REMOTE_CONFIG_APPLIED");
}

export async function fetchRemoteConfig(
  baseURL: string,
  trustedKeys: TrustedRemoteConfigKeys,
  now = new Date(),
): Promise<SignedRemoteConfigEnvelope> {
  const origin = baseURL.replace(/\/+$/, "");
  const response = await fetch(`${origin}/v1/mobile/remote-config`, {
    method: "GET",
    headers: {accept: "application/json"},
  });
  if (!response.ok) {
    throw new Error(`REMOTE_CONFIG_HTTP_${response.status}`);
  }
  const payload: unknown = await response.json();
  if (!verifySignedRemoteConfigEnvelope(payload, trustedKeys, now)) {
    throw new Error("REMOTE_CONFIG_SIGNATURE_OR_PAYLOAD_INVALID");
  }
  return payload;
}

export function verifySignedRemoteConfigEnvelope(
  value: unknown,
  trustedKeys: TrustedRemoteConfigKeys,
  now = new Date(),
): value is SignedRemoteConfigEnvelope {
  return verifySignedRemoteConfigEnvelopeSignature(value, trustedKeys) &&
    isRemoteConfigEnvelopeFresh(value, now);
}

function verifySignedRemoteConfigEnvelopeSignature(
  value: unknown,
  trustedKeys: TrustedRemoteConfigKeys,
): value is SignedRemoteConfigEnvelope {
  if (!isRemoteConfigEnvelopeShape(value)) return false;
  const publicKeyBase64 = trustedKeys[value.key_id];
  if (!publicKeyBase64) return false;
  let signature: Uint8Array;
  let publicKey: Uint8Array;
  try {
    signature = decodeBase64(value.signature);
    publicKey = decodeBase64(publicKeyBase64);
  } catch {
    return false;
  }
  if (
    signature.length !== nacl.sign.signatureLength ||
    publicKey.length !== nacl.sign.publicKeyLength
  ) return false;
  const message = new TextEncoder().encode(canonicalRemoteConfigPayload(value.payload));
  return nacl.sign.detached.verify(message, signature, publicKey);
}

export function isSignedRemoteConfigEnvelope(
  value: unknown,
  now = new Date(),
): value is SignedRemoteConfigEnvelope {
  return isRemoteConfigEnvelopeShape(value) && isRemoteConfigEnvelopeFresh(value, now);
}

function isRemoteConfigEnvelopeShape(value: unknown): value is SignedRemoteConfigEnvelope {
  if (!value || typeof value !== "object") return false;
  const envelope = value as Record<string, unknown>;
  if (
    typeof envelope.key_id !== "string" ||
    envelope.key_id.trim().length === 0 ||
    typeof envelope.signature !== "string" ||
    !signaturePattern.test(envelope.signature) ||
    !envelope.payload ||
    typeof envelope.payload !== "object"
  ) return false;

  const payload = envelope.payload as Record<string, unknown>;
  if (
    !Number.isInteger(payload.version) ||
    Number(payload.version) <= 0 ||
    typeof payload.policy_id !== "string" ||
    payload.policy_id.trim().length === 0 ||
    typeof payload.issued_at !== "string" ||
    typeof payload.expires_at !== "string" ||
    !Array.isArray(payload.disabled_capabilities)
  ) return false;
  const issuedAt = Date.parse(payload.issued_at);
  const expiresAt = Date.parse(payload.expires_at);
  if (!Number.isFinite(issuedAt) || !Number.isFinite(expiresAt) || expiresAt <= issuedAt) return false;

  const disabled = payload.disabled_capabilities;
  const seen = new Set<string>();
  for (const capability of disabled) {
    if (
      typeof capability !== "string" ||
      !remoteCapabilities.includes(capability as RemoteCapability) ||
      seen.has(capability)
    ) return false;
    seen.add(capability);
  }
  if (payload.reason_codes !== undefined) {
    if (!payload.reason_codes || typeof payload.reason_codes !== "object") return false;
    for (const [capability, reason] of Object.entries(payload.reason_codes)) {
      if (
        !remoteCapabilities.includes(capability as RemoteCapability) ||
        !seen.has(capability) ||
        typeof reason !== "string" ||
        reason.trim().length === 0
      ) return false;
    }
  }
  return true;
}

function isRemoteConfigEnvelopeFresh(
  envelope: SignedRemoteConfigEnvelope,
  now: Date,
): boolean {
  return now.getTime() < Date.parse(envelope.payload.expires_at);
}

export function canonicalRemoteConfigPayload(
  payload: RemoteConfigPayload,
): string {
  const ordered: Record<string, unknown> = {
    version: payload.version,
    issued_at: payload.issued_at,
    expires_at: payload.expires_at,
    policy_id: payload.policy_id,
    disabled_capabilities: payload.disabled_capabilities,
  };
  if (payload.reason_codes && Object.keys(payload.reason_codes).length > 0) {
    ordered.reason_codes = Object.fromEntries(
      Object.entries(payload.reason_codes).sort(([left], [right]) =>
        left < right ? -1 : left > right ? 1 : 0,
      ),
    );
  }
  return JSON.stringify(ordered);
}

async function loadCachedState(
  storage: RemoteConfigStorage,
  trustedKeys: TrustedRemoteConfigKeys,
  now: Date,
): Promise<CachedRemoteConfigState | null> {
  try {
    const raw = await storage.load();
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    if (!verifySignedRemoteConfigEnvelopeSignature(parsed, trustedKeys)) {
      await storage.clear();
      return null;
    }
    return {
      verifiedEnvelope: parsed,
      applicableEnvelope: isRemoteConfigEnvelopeFresh(parsed, now) ? parsed : null,
    };
  } catch {
    return null;
  }
}

function failSafe(reasonCode: string): ResolvedRemoteConfig {
  return {
    source: "FAIL_SAFE",
    disabledCapabilities: [...remoteCapabilities],
    reasonCode,
  };
}

function fromEnvelope(
  source: Exclude<RemoteConfigSource, "FAIL_SAFE">,
  envelope: SignedRemoteConfigEnvelope,
  reasonCode: string,
): ResolvedRemoteConfig {
  return {
    source,
    envelope,
    disabledCapabilities: [...envelope.payload.disabled_capabilities],
    reasonCode,
  };
}

function sameSemanticConfig(
  left: SignedRemoteConfigEnvelope,
  right: SignedRemoteConfigEnvelope,
): boolean {
  return (
    left.payload.policy_id === right.payload.policy_id &&
    JSON.stringify(left.payload.disabled_capabilities) ===
      JSON.stringify(right.payload.disabled_capabilities) &&
    JSON.stringify(left.payload.reason_codes ?? {}) ===
      JSON.stringify(right.payload.reason_codes ?? {})
  );
}

function decodeBase64(value: string): Uint8Array {
  if (
    value.length === 0 ||
    value.length % 4 !== 0 ||
    !/^[A-Za-z0-9+/]*={0,2}$/.test(value)
  ) {
    throw new Error("BASE64_INVALID");
  }
  const alphabet =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
  const padding = value.endsWith("==") ? 2 : value.endsWith("=") ? 1 : 0;
  const output = new Uint8Array((value.length / 4) * 3 - padding);
  let offset = 0;
  for (let index = 0; index < value.length; index += 4) {
    const a = alphabet.indexOf(value[index] ?? "");
    const b = alphabet.indexOf(value[index + 1] ?? "");
    const c = value[index + 2] === "=" ? 0 : alphabet.indexOf(value[index + 2] ?? "");
    const d = value[index + 3] === "=" ? 0 : alphabet.indexOf(value[index + 3] ?? "");
    if (a < 0 || b < 0 || c < 0 || d < 0) {
      throw new Error("BASE64_INVALID");
    }
    const block = (a << 18) | (b << 12) | (c << 6) | d;
    if (offset < output.length) output[offset++] = (block >> 16) & 0xff;
    if (offset < output.length) output[offset++] = (block >> 8) & 0xff;
    if (offset < output.length) output[offset++] = block & 0xff;
  }
  return output;
}
