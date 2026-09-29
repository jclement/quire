// Settings → Work items: the URL "AB#2433" and "#2433" link to. Its own
// file so the Settings page stays a list of sections.
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client.ts";
import { queryKeys, useWorkItems } from "../../api/queries.ts";
import { useUi } from "../../keys/UiContext.tsx";
import { noAutofill } from "../../lib/noAutofill.ts";

/** What Azure DevOps calls it — the example the owner is most likely to need. */
const EXAMPLE = "https://dev.azure.com/org/project/_workitems/edit/{id}";

export function WorkItemSettings() {
  const { toast } = useUi();
  const queryClient = useQueryClient();
  const current = useWorkItems();
  const [draft, setDraft] = useState<string | null>(null);
  const saved = current.data?.url_template ?? "";
  const value = draft ?? saved;
  const save = useMutation({
    mutationFn: (template: string) => api.setWorkItems(template),
    onSuccess: (settings) => {
      queryClient.setQueryData(queryKeys.workItems, settings);
      setDraft(null);
      toast(
        settings.url_template
          ? "Work item numbers are now links"
          : "Work item links turned off",
      );
    },
  });
  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-sm font-semibold text-heading">Work items</h2>
      <p className="text-xs text-muted">
        Links <span className="font-mono text-body">AB#2433</span> and{" "}
        <span className="font-mono text-body">#2433</span> in notes and tasks to
        your tracker, with <span className="font-mono text-body">{"{id}"}</span>{" "}
        where the number goes. Leave empty for plain text. A number is never a
        tag.
      </p>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          save.mutate(value.trim());
        }}
        className="flex flex-wrap items-center gap-2"
      >
        <input
          value={value}
          onChange={(event) => setDraft(event.target.value)}
          placeholder={EXAMPLE}
          aria-label="Work item URL"
          {...noAutofill("work-item-url")}
          className="field-bare h-8 min-w-0 flex-1 rounded border border-border bg-raised px-2 font-mono text-xs text-heading outline-none focus:border-accent"
        />
        <button
          type="submit"
          disabled={save.isPending || value.trim() === saved}
          className="flex h-8 items-center rounded border border-border bg-accent px-2.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
        >
          Save work item URL
        </button>
      </form>
    </section>
  );
}
