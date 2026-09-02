// The CodeMirror 6 markdown editor. Uncontrolled: the view is created once
// from initialValue and pushes changes out through onChange; the parent
// (DocumentScreen) owns saving. setValue on the ref exists solely for the
// conflict banner's "take disk" action.
import { autocompletion } from "@codemirror/autocomplete";
import { history, historyKeymap, defaultKeymap } from "@codemirror/commands";
import { markdown, markdownLanguage } from "@codemirror/lang-markdown";
import { languages } from "@codemirror/language-data";
import { EditorState } from "@codemirror/state";
import { EditorView, keymap, placeholder } from "@codemirror/view";
import {
  useEffect,
  useImperativeHandle,
  useRef,
  type Ref,
  type RefObject,
} from "react";
import { makeTagSource, wikilinkSource } from "./completions.ts";
import {
  editorHighlighting,
  editorKeymap,
  editorTheme,
  toggleCheckboxOnLine,
} from "./extensions.ts";
import { imagePasteHandler } from "./imagePaste.ts";
import { tableKeymap, tableTools } from "./tables.ts";

export interface MarkdownEditorHandle {
  /** Replaces the whole buffer (conflict resolution: "take disk"). */
  setValue: (text: string) => void;
  getValue: () => string;
  focus: () => void;
  /** Scrolls a 1-based source line into view — the outline's click target. */
  scrollToLine: (line: number) => void;
}

interface MarkdownEditorProps {
  ref?: Ref<MarkdownEditorHandle>;
  initialValue: string;
  onChange: (value: string) => void;
  /** Cmd/Ctrl+S and the 2s idle debounce land here via the parent. */
  onSave: () => void;
  /** Cmd/Ctrl+Enter: save then return to read mode. */
  onSaveAndExit: () => void;
  onBlur: () => void;
  /** Tag pool for `#` autocomplete (recent/loaded docs; a stub is fine). */
  getTags: () => string[];
  /** 1-based top visible line, on scroll — drives the outline's highlight. */
  onTopLineChange?: (line: number) => void;
}

export function MarkdownEditor({
  ref,
  initialValue,
  onChange,
  onSave,
  onSaveAndExit,
  onBlur,
  getTags,
  onTopLineChange,
}: MarkdownEditorProps) {
  const hostRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  // Callbacks live in a ref so the view (created once) never sees stale ones.
  const callbacksRef = useRef({
    onChange,
    onSave,
    onSaveAndExit,
    onBlur,
    getTags,
    onTopLineChange,
  });
  callbacksRef.current = {
    onChange,
    onSave,
    onSaveAndExit,
    onBlur,
    getTags,
    onTopLineChange,
  };

  useEffect(() => {
    if (!hostRef.current) return;
    const view = new EditorView({
      parent: hostRef.current,
      state: EditorState.create({
        doc: initialValue,
        extensions: buildExtensions(callbacksRef),
      }),
    });
    viewRef.current = view;
    view.focus();
    return () => {
      viewRef.current = null;
      view.destroy();
    };
    // The editor is intentionally created once; later initialValue changes are
    // the parent's business via ref.setValue.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useImperativeHandle(ref, () => ({
    setValue: (text: string) => {
      const view = viewRef.current;
      if (!view) return;
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: text },
      });
    },
    getValue: () => viewRef.current?.state.doc.toString() ?? "",
    focus: () => viewRef.current?.focus(),
    scrollToLine: (line: number) => {
      const view = viewRef.current;
      if (!view) return;
      const clamped = Math.min(Math.max(line, 1), view.state.doc.lines);
      const position = view.state.doc.line(clamped).from;
      view.dispatch({
        effects: EditorView.scrollIntoView(position, { y: "start" }),
      });
    },
  }));

  // `isolate`: CodeMirror's sticky table panel sits at z-index 300, which
  // without its own stacking context paints over every modal in the app.
  return (
    <div ref={hostRef} className="isolate min-h-64 flex-1 overflow-y-auto" />
  );
}

type CallbacksRef = RefObject<{
  onChange: (value: string) => void;
  onSave: () => void;
  onSaveAndExit: () => void;
  onBlur: () => void;
  getTags: () => string[];
  onTopLineChange?: (line: number) => void;
}>;

function buildExtensions(callbacks: CallbacksRef) {
  return [
    history(),
    // GFM base so `- [ ]` task lists parse and Enter continues them; the
    // bundled markdown keymap provides continuation / empty-item exit.
    // codeLanguages gives fenced blocks real per-language highlighting;
    // each grammar is a lazily-imported chunk, so it costs nothing until a
    // note actually contains that language.
    markdown({ base: markdownLanguage, codeLanguages: languages }),
    editorTheme,
    editorHighlighting,
    EditorView.lineWrapping,
    // No smart quotes / autocap on mobile keyboards; spellcheck stays on for prose.
    EditorView.contentAttributes.of({
      autocorrect: "off",
      autocapitalize: "off",
      spellcheck: "true",
    }),
    autocompletion({
      override: [
        wikilinkSource,
        makeTagSource(() => callbacks.current.getTags()),
      ],
    }),
    imagePasteHandler,
    tableTools,
    EditorView.domEventHandlers({
      blur: () => {
        callbacks.current.onBlur();
        return false;
      },
      // Report the top visible line so the outline can highlight the section
      // being edited (lineBlockAtHeight takes scroller-relative coordinates).
      scroll: (_event, view) => {
        const report = callbacks.current.onTopLineChange;
        if (!report) return false;
        const block = view.lineBlockAtHeight(view.scrollDOM.scrollTop);
        report(view.state.doc.lineAt(block.from).number);
        return false;
      },
    }),
    editorKeymap([
      ...tableKeymap,
      {
        key: "Mod-Enter",
        run: () => (callbacks.current.onSaveAndExit(), true),
      },
      {
        key: "Mod-s",
        run: () => (callbacks.current.onSave(), true),
        preventDefault: true,
      },
      { key: "Mod-l", run: toggleCheckboxOnLine },
    ]),
    keymap.of([...defaultKeymap, ...historyKeymap]),
    placeholder("Write…"),
    EditorView.updateListener.of((update) => {
      if (update.docChanged) {
        callbacks.current.onChange(update.state.doc.toString());
      }
    }),
  ];
}
