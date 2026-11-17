"use client";

import { createBrowserApiClient, problemMessage } from "./api/client";
import type { components } from "./api/schema";

export type PasskeySummaryDTO = components["schemas"]["PasskeySummary"];

export interface PasskeyErrorInfo {
  cancelled: boolean;
  message: string;
}

type SignalUnknownFn = (options: {
  rpId: string;
  credentialId: string;
}) => Promise<void>;

interface PublicKeyCredentialWithModernAPIs {
  parseCreationOptionsFromJSON?: (
    json: Record<string, unknown>,
  ) => PublicKeyCredentialCreationOptions;
  parseRequestOptionsFromJSON?: (
    json: Record<string, unknown>,
  ) => PublicKeyCredentialRequestOptions;
  signalUnknownCredential?: SignalUnknownFn;
}

function getModernPKC(): PublicKeyCredentialWithModernAPIs {
  if (typeof PublicKeyCredential === "undefined") {
    return {};
  }
  return PublicKeyCredential as unknown as PublicKeyCredentialWithModernAPIs;
}

export function supportsPasskeys(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof PublicKeyCredential !== "undefined" &&
    typeof navigator !== "undefined" &&
    Boolean(navigator.credentials)
  );
}

export function unwrapPublicKeyOptions(
  raw: Record<string, unknown>,
): Record<string, unknown> {
  const nested = raw.publicKey;
  if (nested && typeof nested === "object" && !Array.isArray(nested)) {
    return nested as Record<string, unknown>;
  }
  return raw;
}

function base64UrlToBuffer(value: string): ArrayBuffer {
  const padded = value.replace(/-/g, "+").replace(/_/g, "/");
  const padLen = (4 - (padded.length % 4)) % 4;
  const bin = atob(padded + "=".repeat(padLen));
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i += 1) {
    bytes[i] = bin.charCodeAt(i);
  }
  return bytes.buffer;
}

function bufferToBase64Url(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf);
  let bin = "";
  for (const b of bytes) {
    bin += String.fromCharCode(b);
  }
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function decodeCredentialDescriptors(
  list: unknown,
): PublicKeyCredentialDescriptor[] | undefined {
  if (!Array.isArray(list)) {
    return undefined;
  }
  return list.map((item) => {
    const desc = item as {
      type: PublicKeyCredentialType;
      id: string;
      transports?: AuthenticatorTransport[];
    };
    return {
      type: desc.type,
      id: base64UrlToBuffer(desc.id),
      ...(desc.transports ? { transports: desc.transports } : {}),
    };
  });
}

export function parseCreationOptions(
  raw: Record<string, unknown>,
): PublicKeyCredentialCreationOptions {
  const pkJson = unwrapPublicKeyOptions(raw);
  const modern = getModernPKC();
  if (typeof modern.parseCreationOptionsFromJSON === "function") {
    return modern.parseCreationOptionsFromJSON(pkJson);
  }

  const user = pkJson.user as {
    id: string;
    name: string;
    displayName: string;
  };
  return {
    ...(pkJson as unknown as PublicKeyCredentialCreationOptions),
    challenge: base64UrlToBuffer(String(pkJson.challenge)),
    user: {
      ...user,
      id: base64UrlToBuffer(user.id),
    },
    ...(pkJson.excludeCredentials
      ? {
          excludeCredentials: decodeCredentialDescriptors(
            pkJson.excludeCredentials,
          ),
        }
      : {}),
  };
}

export function parseRequestOptions(
  raw: Record<string, unknown>,
): PublicKeyCredentialRequestOptions {
  const pkJson = unwrapPublicKeyOptions(raw);
  const modern = getModernPKC();
  if (typeof modern.parseRequestOptionsFromJSON === "function") {
    return modern.parseRequestOptionsFromJSON(pkJson);
  }

  return {
    ...(pkJson as unknown as PublicKeyCredentialRequestOptions),
    challenge: base64UrlToBuffer(String(pkJson.challenge)),
    ...(pkJson.allowCredentials
      ? {
          allowCredentials: decodeCredentialDescriptors(
            pkJson.allowCredentials,
          ),
        }
      : {}),
  };
}

export function serializeCredential(
  credential: PublicKeyCredential,
): Record<string, unknown> {
  const withToJSON = credential as PublicKeyCredential & {
    toJSON?: () => Record<string, unknown>;
  };
  if (typeof withToJSON.toJSON === "function") {
    return withToJSON.toJSON();
  }

  const base: Record<string, unknown> = {
    id: credential.id,
    rawId: bufferToBase64Url(credential.rawId),
    type: credential.type,
  };
  const resp = credential.response;
  if ("attestationObject" in resp) {
    const att = resp as AuthenticatorAttestationResponse;
    base.response = {
      clientDataJSON: bufferToBase64Url(att.clientDataJSON),
      attestationObject: bufferToBase64Url(att.attestationObject),
      ...(typeof att.getTransports === "function"
        ? { transports: att.getTransports() }
        : {}),
    };
  } else {
    const ass = resp as AuthenticatorAssertionResponse;
    base.response = {
      clientDataJSON: bufferToBase64Url(ass.clientDataJSON),
      authenticatorData: bufferToBase64Url(ass.authenticatorData),
      signature: bufferToBase64Url(ass.signature),
      ...(ass.userHandle
        ? { userHandle: bufferToBase64Url(ass.userHandle) }
        : {}),
    };
  }
  return base;
}

export async function signalUnknownCredentialIfSupported(
  rpId: string | undefined,
  credentialId: string | undefined,
): Promise<void> {
  if (!rpId || !credentialId) {
    return;
  }
  const modern = getModernPKC();
  if (typeof modern.signalUnknownCredential === "function") {
    try {
      await modern.signalUnknownCredential({ rpId, credentialId });
    } catch {
      // Ignored: signal sync is a progressive optimization.
    }
  }
}

export function describePasskeyError(err: unknown): PasskeyErrorInfo {
  if (err instanceof DOMException) {
    if (err.name === "NotAllowedError" || err.name === "AbortError") {
      return {
        cancelled: true,
        message: "Operación de passkey cancelada o expirada.",
      };
    }
    if (err.name === "InvalidStateError") {
      return {
        cancelled: false,
        message: "Este dispositivo ya tiene una passkey registrada.",
      };
    }
    if (err.name === "SecurityError") {
      return {
        cancelled: false,
        message:
          "El dominio actual no coincide con el configurado para passkeys.",
      };
    }
  }
  return {
    cancelled: false,
    message: "No se pudo completar la verificación con passkey.",
  };
}

export async function registerPasskeyForSession(
  csrfToken: string,
): Promise<
  { ok: true; passkey: PasskeySummaryDTO } | { ok: false; message: string }
> {
  if (!supportsPasskeys()) {
    return {
      ok: false,
      message: "Este navegador no admite passkeys.",
    };
  }

  const api = createBrowserApiClient();
  const { data: beginData, error: beginErr } = await api.POST(
    "/auth/passkeys/register/begin",
    {
      headers: { "X-CSRF-Token": csrfToken },
    },
  );
  if (beginErr || !beginData) {
    return {
      ok: false,
      message: problemMessage(
        beginErr,
        "No se pudo iniciar el registro de passkey.",
      ),
    };
  }

  const pkJson = unwrapPublicKeyOptions(beginData.options);
  const rpObj = pkJson.rp as { id?: string } | undefined;
  const rpId = typeof rpObj?.id === "string" ? rpObj.id : undefined;
  const publicKey = parseCreationOptions(beginData.options);

  let credential: Credential | null = null;
  try {
    credential = await navigator.credentials.create({ publicKey });
  } catch (err) {
    return {
      ok: false,
      message: describePasskeyError(err).message,
    };
  }

  if (!credential || credential.type !== "public-key") {
    return {
      ok: false,
      message: "El navegador no devolvió una credencial válida.",
    };
  }

  const encoded = serializeCredential(credential as PublicKeyCredential);
  try {
    const { data: finishData, error: finishErr } = await api.POST(
      "/auth/passkeys/register/finish",
      {
        headers: { "X-CSRF-Token": csrfToken },
        body: {
          challengeId: beginData.challengeId,
          credential: encoded,
        },
      },
    );
    if (finishErr || !finishData) {
      await signalUnknownCredentialIfSupported(
        rpId,
        typeof encoded.id === "string" ? encoded.id : undefined,
      );
      return {
        ok: false,
        message: problemMessage(finishErr, "No se pudo guardar la passkey."),
      };
    }
    return { ok: true, passkey: finishData };
  } catch {
    await signalUnknownCredentialIfSupported(
      rpId,
      typeof encoded.id === "string" ? encoded.id : undefined,
    );
    return {
      ok: false,
      message: "Error de red al guardar la passkey.",
    };
  }
}
