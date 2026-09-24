<script lang="ts">
  import type { Snippet } from 'svelte'
  import Icon, { type IconName } from './Icon.svelte'

  let {
    icon = 'calendar',
    title,
    text = '',
    tone = 'plain',
    children,
  }: { icon?: IconName; title: string; text?: string; tone?: 'plain' | 'bad'; children?: Snippet } = $props()
</script>

<div class="empty" class:bad={tone === 'bad'} role={tone === 'bad' ? 'alert' : undefined}>
  <div class="mark"><Icon name={icon} size={22} /></div>
  <h2>{title}</h2>
  {#if text}<p>{text}</p>{/if}
  {#if children}<div class="row actions">{@render children()}</div>{/if}
</div>

<style>
  .empty {
    display: grid; justify-items: center; gap: 8px; text-align: center; padding: 40px 20px;
    background: var(--surface); border: 1px dashed var(--line-strong); border-radius: var(--radius);
  }
  .mark {
    width: 48px; height: 48px; border-radius: 14px; display: grid; place-items: center;
    background: var(--accent-soft); color: var(--accent); margin-bottom: 4px;
  }
  .bad .mark { background: var(--bad-soft); color: var(--bad); }
  p { color: var(--ink-2); max-width: 40ch; }
  .actions { justify-content: center; margin-top: 6px; }
</style>
