// Attributes that stop browsers and password managers from decorating quire's
// own text inputs.
//
// `autoComplete="off"` alone is not enough: Safari and Chrome fall back to
// field heuristics driven by the input's name/id/placeholder, which is how a
// "Company title…" box ends up offering macOS Contacts entries. A name the
// heuristics don't recognise is what actually turns them off, so every field
// gets a unique nonsense name; the rest silences iOS autocorrect/caps and the
// 1Password / LastPass inline icons.

export interface NoAutofillProps {
  name: string;
  autoComplete: "off";
  autoCorrect: "off";
  autoCapitalize: "off";
  spellCheck: false;
  "data-1p-ignore": string;
  "data-lpignore": "true";
  "data-form-type": "other";
}

/**
 * Spread onto an <input>. `field` only has to be unique within the app — it is
 * deliberately not a guessable name like "title" or "company".
 */
export function noAutofill(field: string): NoAutofillProps {
  return {
    name: `quire-${field}-x`,
    autoComplete: "off",
    autoCorrect: "off",
    autoCapitalize: "off",
    spellCheck: false,
    "data-1p-ignore": "",
    "data-lpignore": "true",
    "data-form-type": "other",
  };
}
