// WebAuthn ceremony wrappers: begin → browser credential API → finish, with
// @github/webauthn-json handling the base64url field encoding. client.ts stays
// pure HTTP; these are the only functions the auth UI calls.
import {
  create,
  get,
  type CredentialCreationOptionsJSON,
  type CredentialRequestOptionsJSON,
} from "@github/webauthn-json";
import { api } from "./client.ts";

/** The server may send bare PublicKeyCredential*Options or the {publicKey:…}
 * wrapper the browser API wants; accept either. */
function wrapPublicKey<T>(options: Record<string, unknown>): T {
  return ("publicKey" in options ? options : { publicKey: options }) as T;
}

/**
 * Registers a new passkey named `label`. Returns the recovery codes when this
 * was the very first passkey (show them immediately — they are never shown
 * again), else null. Also establishes the session cookie.
 */
export async function registerPasskey(label: string): Promise<string[] | null> {
  const options = await api.authRegisterBegin();
  const credential = await create(
    wrapPublicKey<CredentialCreationOptionsJSON>(options),
  );
  const result = await api.authRegisterFinish(label, credential);
  return result.recovery_codes;
}

/** Full passkey sign-in ceremony; resolves once the session cookie is set. */
export async function loginWithPasskey(): Promise<void> {
  const options = await api.authLoginBegin();
  const assertion = await get(
    wrapPublicKey<CredentialRequestOptionsJSON>(options),
  );
  await api.authLoginFinish(assertion);
}
