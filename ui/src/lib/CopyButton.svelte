<script lang="ts">
  import Icon from "./Icon.svelte";
  import { notices } from "./notice.svelte";

  // Copies a text, and says so. With a label it is a button with text; without, an icon.
  let { text, label, title }: { text: string; label?: string; title?: string } = $props();

  let copied = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;

  async function copy(e: MouseEvent) {
    e.stopPropagation(); // a row or a card that holds the button does not see the click
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      // the clipboard of the browser is not open to this page: use the old way
      const area = document.createElement("textarea");
      area.value = text;
      document.body.append(area);
      area.select();
      const ok = document.execCommand("copy");
      area.remove();
      if (!ok) {
        notices.error("The browser did not let Dolly copy. The text is: " + text);
        return;
      }
    }
    copied = true;
    clearTimeout(timer);
    timer = setTimeout(() => (copied = false), 1500);
  }
</script>

<button type="button" class={["btn", label ? "" : "icon ghost"]} title={title ?? text} onclick={copy}>
  <Icon name={copied ? "check" : "copy"} />
  {#if label}{copied ? "Copied" : label}{/if}
  <span class="sr-only" aria-live="polite">{copied ? "Copied" : ""}</span>
</button>
