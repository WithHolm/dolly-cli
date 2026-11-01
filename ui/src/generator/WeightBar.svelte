<script lang="ts">
  import Icon from "../lib/Icon.svelte";

  // A bar that sets a number: drag it, press the arrow keys, or type in the box. The bar is the share of a scale that does not move while
  // you drag (2, 5, 10, 20, 50, 100 ... up to the biggest value), so a bar next to another one shows which is bigger. A lock keeps the value:
  // a locked bar cannot be set, and the list does not take from it.

  let {
    value,
    label,
    scale,
    min = 1,
    max = 1000,
    unit = "",
    divisor = 1,
    locked = false,
    onlock,
    onstart,
    onend,
    onchange,
  }: {
    value: number;
    label: string;
    scale: number;
    min?: number;
    max?: number;
    unit?: string;
    /** The value is this many times the number that is shown: 10 shows 343 as 34.3. */
    divisor?: number;

    locked?: boolean;
    onlock?: () => void;
    onstart?: () => void;
    onend?: () => void;
    onchange: (n: number) => void;
  } = $props();

  let track: HTMLElement;
  let dragging = $state(false);
  // while a bar is dragged its scale is held: when it is the biggest one, the scale of the list would shrink under the pointer
  let held = $state<number | null>(null);
  const s = $derived(held ?? scale);
  const share = $derived(Math.min(100, (value / s) * 100));
  const clamp = (n: number) => Math.max(min, Math.min(max, Math.round(n)));
  const text = $derived(divisor === 1 ? String(value) : (value / divisor).toFixed(1));

  function typed(e: Event & { currentTarget: HTMLInputElement }) {
    const n = Number.parseFloat(e.currentTarget.value.replace(",", "."));
    if (Number.isNaN(n)) e.currentTarget.value = text; // not a number: show the value again
    else onchange(clamp(n * divisor));
  }

  function fromPointer(e: PointerEvent) {
    const r = track.getBoundingClientRect();
    onchange(clamp(((e.clientX - r.left) / r.width) * s));
  }

  function down(e: PointerEvent) {
    if (locked) return;
    dragging = true;
    held = scale;
    onstart?.();
    track.setPointerCapture(e.pointerId);
    fromPointer(e);
  }

  function up() {
    if (!dragging) return;
    dragging = false;
    held = null;
    onend?.();
  }

  function key(e: KeyboardEvent) {
    if (locked) return;
    const step = (e.shiftKey ? 10 : 1) * divisor; // one percent, or ten with Shift
    const to = { ArrowRight: value + step, ArrowUp: value + step, ArrowLeft: value - step, ArrowDown: value - step, Home: min, End: s }[e.key];
    if (to === undefined) return;
    e.preventDefault();
    onchange(clamp(to));
  }
</script>

<div class="weight" class:locked>
  <div
    bind:this={track}
    class="track"
    class:dragging
    role="slider"
    tabindex="0"
    aria-label={label}
    aria-valuemin={min / divisor}
    aria-valuemax={s / divisor}
    aria-valuenow={value / divisor}
    aria-valuetext="{text}{unit}"
    aria-disabled={locked || undefined}
    onpointerdown={down}
    onpointermove={(e) => dragging && fromPointer(e)}
    onpointerup={up}
    onpointercancel={up}
    onkeydown={key}
  >
    <div class="fill" style:width="{share}%"></div>
  </div>
  <input type="text" inputmode="decimal" aria-label="{label}, number" value={text} disabled={locked} onchange={typed} />
  {#if unit}<span class="unit">{unit}</span>{/if}
  {#if onlock}
    <button class="lock" class:on={locked} aria-pressed={locked} aria-label="Lock the share: {label}" title={locked ? "Locked: the others cannot take from it. Click to unlock." : "Lock this share"} onclick={onlock}>
      <Icon name="lock" size={14} />
    </button>
  {/if}
</div>

<style>
  .weight { display: flex; align-items: center; gap: 8px; min-width: 0; }
  .track { flex: 1; height: 28px; min-width: 80px; border-radius: var(--radius); background: var(--sunken); cursor: ew-resize; touch-action: none; overflow: hidden; }
  .locked .track { cursor: not-allowed; }
  .track:hover .fill, .track.dragging .fill { background: color-mix(in srgb, var(--primary) 35%, var(--primary-soft)); }
  .locked .track:hover .fill { background: var(--primary-soft); }
  .fill { height: 100%; background: var(--primary-soft); border-right: 2px solid var(--primary); transition: width 0.05s; }
  .locked .fill { background: repeating-linear-gradient(135deg, var(--primary-soft) 0 6px, color-mix(in srgb, var(--primary) 18%, var(--primary-soft)) 6px 12px); border-right-color: var(--muted); }
  input { width: 56px; text-align: right; font-variant-numeric: tabular-nums; }
  .unit { color: var(--muted); }
  .lock { display: inline-grid; place-items: center; width: 28px; height: 28px; padding: 0; border: 1px solid transparent; border-radius: var(--radius); background: none; color: var(--faint); cursor: pointer; }
  .lock:hover { background: var(--sunken); color: var(--text); }
  .lock.on { color: var(--primary); background: var(--primary-soft); border-color: var(--primary); }
</style>
