<script lang="ts">
  import { toast } from '../toast.svelte'
</script>

<div class="toast-region" role="status" aria-live="polite">
  {#if toast.message}
    <div class="toast" class:bad={toast.tone === 'bad'}>{toast.message}</div>
  {/if}
</div>

<style>
  .toast-region {
    position: fixed; left: 16px; right: 16px; bottom: calc(84px + env(safe-area-inset-bottom, 0px));
    z-index: 60; display: flex; justify-content: center; pointer-events: none;
  }
  .toast {
    background: var(--ink); color: var(--bg); padding: 10px 16px; border-radius: 8px; font-size: 14px; font-weight: 600;
    box-shadow: var(--shadow-lg); max-width: 480px; animation: pop .18s ease-out;
  }
  .toast.bad { background: var(--bad-soft); color: var(--bad); border: 1px solid var(--bad); }
  @media (min-width: 761px) { .toast-region { bottom: 28px; } }
  @keyframes pop { from { transform: translateY(8px); opacity: 0; } }
</style>
