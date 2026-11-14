<script lang="ts">
  import { theme, type ThemeChoice } from '../theme.svelte'
  import Icon from './Icon.svelte'

  // Light, dark, or as the Mac is set. It used to sit at the foot of the
  // sidebar. FrameFairy keeps how the app looks in its settings, under
  // Appearance, and so does this app.
  const CHOICES: { value: ThemeChoice; label: string; icon: 'auto' | 'sun' | 'moon' }[] = [
    { value: 'system', label: 'System', icon: 'auto' },
    { value: 'light', label: 'Light', icon: 'sun' },
    { value: 'dark', label: 'Dark', icon: 'moon' },
  ]
</script>

<section class="group">
  <h2>Appearance</h2>
  <div class="list">
    <div class="setting">
      <div class="info">
        <span class="desc" id="theme-label">Theme</span>
        <span class="state small">{theme.choice === 'system' ? 'Follows the Mac' : theme.choice === 'light' ? 'Always light' : 'Always dark'}</span>
      </div>
      <div class="segmented" role="group" aria-labelledby="theme-label">
        {#each CHOICES as c (c.value)}
          <button type="button" aria-pressed={theme.choice === c.value} onclick={() => theme.set(c.value)}>
            <Icon name={c.icon} size={14} />{c.label}
          </button>
        {/each}
      </div>
    </div>
  </div>
</section>

<style>
  .group { display: grid; gap: 10px; }
  .group h2 { font-size: 15px; margin: 0; }
  /* The same row as every other setting, see SettingRow. */
  .setting {
    display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 4px 24px; padding: 14px 16px; align-items: center;
  }
  .info { display: grid; gap: 2px; min-width: 0; }
  .desc { font-weight: 500; font-size: 14px; }
  .state { color: var(--ink-3); }
  .segmented button { display: inline-flex; align-items: center; gap: 6px; min-width: 6.2em; justify-content: center; }
  @media (max-width: 760px) {
    .setting { grid-template-columns: minmax(0, 1fr); }
  }
</style>
