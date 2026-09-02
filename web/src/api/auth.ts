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
// enrollCode is required only when claiming an un-bootstrapped instance that
// listens on a non-loopback address; the server prints it at startup. Both
// legs of the ceremony carry it, because both check.
export async function registerPasskey(
  label: string,
  enrollCode?: string,
): Promise<string[] | null> {
  const options = await api.authRegisterBegin(enrollCode);
  const credential = await create(
    wrapPublicKey<CredentialCreationOptionsJSON>(options),
  );
  const result = await api.authRegisterFinish(label, credential, enrollCode);
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
